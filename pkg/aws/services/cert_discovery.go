package services

import (
	"context"
	"errors"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/acm"
	"k8s.io/apimachinery/pkg/util/cache"
)

const (
	certListCacheKey = "certList"
	certListCacheTTL = 1 * time.Minute
	disabledCacheKey = "disabled"
	disabledCacheTTL = 5 * time.Minute
	cacheJitter      = 10 * time.Second

	// DescribeCertificate cache TTLs, matching ALB controller values.
	// Only used for certs with >100 SANs (HasAdditionalSubjectAlternativeNames=true).
	importedCertDomainsCacheTTL = 5 * time.Minute
	privateCertDomainsCacheTTL  = 10 * time.Hour
)
our implementation returns the error (triggering
requeue)
// CertificateDiscovery discovers ACM certificates for hostnames.
type CertificateDiscovery interface {
	Discover(ctx context.Context, hostname string) (string, error)
}

// jitteredTTL adds random jitter of [0, cacheJitter) to a base TTL.
func jitteredTTL(base time.Duration) time.Duration {
	return base + time.Duration(rand.Int63n(int64(cacheJitter)))
}

type certDiscovery struct {
	acm          ACM
	mu           sync.Mutex
	cache        *cache.Expiring
	domainsCache *cache.Expiring // per-ARN domain cache for truncated SAN certs
}

// NewCertificateDiscovery creates a new CertificateDiscovery instance.
func NewCertificateDiscovery(acmClient ACM) CertificateDiscovery {
	return &certDiscovery{
		acm:          acmClient,
		cache:        cache.NewExpiring(),
		domainsCache: cache.NewExpiring(),
	}
}

// Discover finds the best matching ACM certificate for the given hostname.
// Returns the certificate ARN or empty string if no match is found.
func (d *certDiscovery) Discover(ctx context.Context, hostname string) (string, error) {
	if _, ok := d.cache.Get(disabledCacheKey); ok {
		return "", nil
	}

	certs, err := d.loadCertificates(ctx)
	if err != nil {
		if errors.Is(err, ErrACMAccessDenied) {
			d.cache.Set(disabledCacheKey, true, disabledCacheTTL)
			return "", nil
		}
		return "", err
	}

	return d.findBestMatch(ctx, certs, hostname)
}

// loadCertificates returns the cached cert list or fetches from ACM on cache miss.
func (d *certDiscovery) loadCertificates(ctx context.Context) ([]*acm.CertificateSummary, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if val, ok := d.cache.Get(certListCacheKey); ok {
		return val.([]*acm.CertificateSummary), nil
	}

	certs, err := d.acm.ListCertificatesAsList(ctx, &acm.ListCertificatesInput{
		CertificateStatuses: []*string{aws.String(acm.CertificateStatusIssued)},
	})
	if err != nil {
		return nil, err
	}

	d.cache.Set(certListCacheKey, certs, jitteredTTL(certListCacheTTL))
	return certs, nil
}

// findBestMatch picks the best matching certificate for a hostname.
// Priority: exact match > wildcard match, then most recent timestamp within the same tier.
func (d *certDiscovery) findBestMatch(ctx context.Context, certs []*acm.CertificateSummary, hostname string) (string, error) {
	var exactMatches, wildcardMatches []*acm.CertificateSummary

	for _, cert := range certs {
		domains, err := d.getDomainsForCert(ctx, cert)
		if err != nil {
			return "", err
		}
		for _, domain := range domains {
			if matchesExact(domain, hostname) {
				exactMatches = append(exactMatches, cert)
				break
			} else if isWildcardMatch(domain, hostname) {
				wildcardMatches = append(wildcardMatches, cert)
				break
			}
		}
	}

	if best := mostRecentlyIssued(exactMatches); best != nil {
		return aws.StringValue(best.CertificateArn), nil
	}
	if best := mostRecentlyIssued(wildcardMatches); best != nil {
		return aws.StringValue(best.CertificateArn), nil
	}
	return "", nil
}

// getDomainsForCert returns all domain names for a certificate.
// Uses SubjectAlternativeNameSummaries from ListCertificates when complete,
// falls back to DescribeCertificate when the SAN list is truncated (>100 SANs).
func (d *certDiscovery) getDomainsForCert(ctx context.Context, cert *acm.CertificateSummary) ([]string, error) {
	if len(cert.SubjectAlternativeNameSummaries) > 0 &&
		!aws.BoolValue(cert.HasAdditionalSubjectAlternativeNames) {
		return aws.StringValueSlice(cert.SubjectAlternativeNameSummaries), nil
	}

	// Truncated SAN list — use cached domains or fetch via DescribeCertificate
	if aws.BoolValue(cert.HasAdditionalSubjectAlternativeNames) {
		certARN := aws.StringValue(cert.CertificateArn)
		if val, ok := d.domainsCache.Get(certARN); ok {
			return val.([]string), nil
		}

		resp, err := d.acm.DescribeCertificate(ctx, &acm.DescribeCertificateInput{
			CertificateArn: cert.CertificateArn,
		})
		if err != nil {
			return nil, err
		}

		domains := aws.StringValueSlice(resp.Certificate.SubjectAlternativeNames)
		ttl := importedCertDomainsCacheTTL
		if aws.StringValue(cert.Type) != acm.CertificateTypeImported {
			ttl = privateCertDomainsCacheTTL
		}
		d.domainsCache.Set(certARN, domains, jitteredTTL(ttl))
		return domains, nil
	}

	// Fallback: use DomainName if no SANs available
	if cert.DomainName != nil {
		return []string{aws.StringValue(cert.DomainName)}, nil
	}
	return nil, nil
}

// matchesExact returns true if the certificate domain matches the hostname exactly (case-insensitive).
func matchesExact(certDomain, hostname string) bool {
	return strings.EqualFold(certDomain, hostname)
}

// isWildcardMatch returns true if the certificate domain is a valid wildcard pattern
// that matches the hostname. Only single sublevel matching is allowed:
// *.example.com matches foo.example.com but not foo.bar.example.com or example.com.
func isWildcardMatch(certDomain, hostname string) bool {
	if !isValidWildcard(certDomain) {
		return false
	}

	suffix := certDomain[1:] // ".example.com"
	hostname = strings.ToLower(hostname)
	suffix = strings.ToLower(suffix)

	if !strings.HasSuffix(hostname, suffix) {
		return false
	}

	prefix := hostname[:len(hostname)-len(suffix)]
	return len(prefix) > 0 && !strings.Contains(prefix, ".")
}

// isValidWildcard checks that the domain is a well-formed wildcard: exactly "*.something".
func isValidWildcard(domain string) bool {
	if !strings.HasPrefix(domain, "*.") {
		return false
	}
	rest := domain[2:]
	return len(rest) > 0 && !strings.Contains(rest, "*")
}

// mostRecentlyIssued returns the certificate with the most recent relevant timestamp.
func mostRecentlyIssued(certs []*acm.CertificateSummary) *acm.CertificateSummary {
	if len(certs) == 0 {
		return nil
	}
	best := certs[0]
	bestTime := certTimestamp(best)
	for _, cert := range certs[1:] {
		t := certTimestamp(cert)
		if bestTime == nil || (t != nil && t.After(*bestTime)) {
			best = cert
			bestTime = t
		}
	}
	return best
}

// certTimestamp returns the best available timestamp: IssuedAt > ImportedAt > CreatedAt.
func certTimestamp(cert *acm.CertificateSummary) *time.Time {
	if cert.IssuedAt != nil {
		return cert.IssuedAt
	}
	if cert.ImportedAt != nil {
		return cert.ImportedAt
	}
	if cert.CreatedAt != nil {
		return cert.CreatedAt
	}
	return nil
}
