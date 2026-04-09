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
)

type CertificateDiscovery interface {
	Discover(ctx context.Context, hostname string) (string, error)
}

func jitteredTTL(base time.Duration) time.Duration {
	return base + time.Duration(rand.Int63n(int64(cacheJitter)))
}

type certDiscovery struct {
	acm   ACM
	mu    sync.Mutex
	cache *cache.Expiring
}

func NewCertificateDiscovery(acmClient ACM) CertificateDiscovery {
	return &certDiscovery{
		acm:   acmClient,
		cache: cache.NewExpiring(),
	}
}

// Discover finds the best matching ACM certificate for the given hostname.
// Returns the certificate ARN or empty string if no match is found.
func (d *certDiscovery) Discover(ctx context.Context, hostname string) (string, error) {
	if _, ok := d.cache.Get(disabledCacheKey); ok {
		return "", ErrACMAccessDenied
	}

	certs, err := d.loadCertificates(ctx)
	if err != nil {
		if errors.Is(err, ErrACMAccessDenied) {
			d.cache.Set(disabledCacheKey, true, disabledCacheTTL)
			return "", ErrACMAccessDenied
		}
		return "", err
	}

	return d.findBestMatch(certs, hostname), nil
}

// loadCertificates returns the cached cert list or fetches from ACM on cache miss.
func (d *certDiscovery) loadCertificates(ctx context.Context) ([]*acm.CertificateSummary, error) {
	if val, ok := d.cache.Get(certListCacheKey); ok {
		return val.([]*acm.CertificateSummary), nil
	}

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
func (d *certDiscovery) findBestMatch(certs []*acm.CertificateSummary, hostname string) string {
	var exactMatches, wildcardMatches []*acm.CertificateSummary

	for _, cert := range certs {
		domains := getDomainsForCert(cert)
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
		return aws.StringValue(best.CertificateArn)
	}
	if best := mostRecentlyIssued(wildcardMatches); best != nil {
		return aws.StringValue(best.CertificateArn)
	}
	return ""
}

// getDomainsForCert returns all domain names for a certificate.
// Uses SubjectAlternativeNameSummaries from ListCertificates (up to 100 SANs),
// falling back to DomainName if no SANs are available.
func getDomainsForCert(cert *acm.CertificateSummary) []string {
	if len(cert.SubjectAlternativeNameSummaries) > 0 {
		return aws.StringValueSlice(cert.SubjectAlternativeNameSummaries)
	}
	if cert.DomainName != nil {
		return []string{aws.StringValue(cert.DomainName)}
	}
	return nil
}

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

func isValidWildcard(domain string) bool {
	if !strings.HasPrefix(domain, "*.") {
		return false
	}
	rest := domain[2:]
	return len(rest) > 0 && !strings.Contains(rest, "*")
}

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
