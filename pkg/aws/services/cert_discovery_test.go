package services

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/acm"
	"github.com/stretchr/testify/assert"
)

type fakeACM struct {
	certs          []*acm.CertificateSummary
	listErr        error
	listCallCount  int
	describeCerts  map[string]*acm.CertificateDetail
	describeErr    error
	descCallCount  int
	mu             sync.Mutex
}

func (f *fakeACM) ListCertificatesAsList(_ context.Context, _ *acm.ListCertificatesInput) ([]*acm.CertificateSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listCallCount++
	return f.certs, f.listErr
}

func (f *fakeACM) DescribeCertificate(_ context.Context, input *acm.DescribeCertificateInput) (*acm.DescribeCertificateOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.descCallCount++
	if f.describeErr != nil {
		return nil, f.describeErr
	}
	detail, ok := f.describeCerts[aws.StringValue(input.CertificateArn)]
	if !ok {
		return nil, fmt.Errorf("cert not found: %s", aws.StringValue(input.CertificateArn))
	}
	return &acm.DescribeCertificateOutput{Certificate: detail}, nil
}

func TestDiscover_ExactMatchViaSAN(t *testing.T) {
	now := time.Now()
	cert := &acm.CertificateSummary{
		CertificateArn:                     aws.String("arn:exact"),
		DomainName:                         aws.String("example.com"),
		SubjectAlternativeNameSummaries:     []*string{aws.String("example.com"), aws.String("api.example.com")},
		HasAdditionalSubjectAlternativeNames: aws.Bool(false),
		IssuedAt:                           &now,
	}
	fake := &fakeACM{certs: []*acm.CertificateSummary{cert}}
	d := NewCertificateDiscovery(fake)

	// hostname only in SANs, not the primary DomainName
	result, err := d.Discover(context.Background(), "api.example.com")
	assert.NoError(t, err)
	assert.Equal(t, "arn:exact", result)
}

func TestDiscover_GlobalCertListCache(t *testing.T) {
	now := time.Now()
	cert1 := &acm.CertificateSummary{
		CertificateArn:                     aws.String("arn:cert-api"),
		DomainName:                         aws.String("api.example.com"),
		SubjectAlternativeNameSummaries:     []*string{aws.String("api.example.com")},
		HasAdditionalSubjectAlternativeNames: aws.Bool(false),
		IssuedAt:                           &now,
	}
	cert2 := &acm.CertificateSummary{
		CertificateArn:                     aws.String("arn:cert-web"),
		DomainName:                         aws.String("web.example.com"),
		SubjectAlternativeNameSummaries:     []*string{aws.String("web.example.com")},
		HasAdditionalSubjectAlternativeNames: aws.Bool(false),
		IssuedAt:                           &now,
	}
	fake := &fakeACM{certs: []*acm.CertificateSummary{cert1, cert2}}
	d := NewCertificateDiscovery(fake)

	// Two different hostnames should share one ListCertificates call
	r1, err := d.Discover(context.Background(), "api.example.com")
	assert.NoError(t, err)
	assert.Equal(t, "arn:cert-api", r1)

	r2, err := d.Discover(context.Background(), "web.example.com")
	assert.NoError(t, err)
	assert.Equal(t, "arn:cert-web", r2)

	assert.Equal(t, 1, fake.listCallCount, "expected single ListCertificates call for both hostnames")
}

func TestDiscover_PermissionError_TimedDisable(t *testing.T) {
	fake := &fakeACM{listErr: ErrACMAccessDenied}
	d := NewCertificateDiscovery(fake)

	// First call triggers timed disable
	result, err := d.Discover(context.Background(), "api.example.com")
	assert.NoError(t, err)
	assert.Empty(t, result)
	assert.Equal(t, 1, fake.listCallCount)

	// Second call short-circuits without calling ACM
	result, err = d.Discover(context.Background(), "other.example.com")
	assert.NoError(t, err)
	assert.Empty(t, result)
	assert.Equal(t, 1, fake.listCallCount)
}

func TestDiscover_EmptyResultCaching(t *testing.T) {
	fake := &fakeACM{certs: []*acm.CertificateSummary{}}
	d := NewCertificateDiscovery(fake)

	r1, err := d.Discover(context.Background(), "unknown.example.com")
	assert.NoError(t, err)
	assert.Empty(t, r1)

	// Second call uses cached cert list
	r2, err := d.Discover(context.Background(), "unknown.example.com")
	assert.NoError(t, err)
	assert.Empty(t, r2)
	assert.Equal(t, 1, fake.listCallCount)
}

func TestDiscover_WildcardMatchViaSAN(t *testing.T) {
	now := time.Now()
	cert := &acm.CertificateSummary{
		CertificateArn:                     aws.String("arn:wildcard"),
		DomainName:                         aws.String("*.example.com"),
		SubjectAlternativeNameSummaries:     []*string{aws.String("*.example.com")},
		HasAdditionalSubjectAlternativeNames: aws.Bool(false),
		IssuedAt:                           &now,
	}
	fake := &fakeACM{certs: []*acm.CertificateSummary{cert}}
	d := NewCertificateDiscovery(fake)

	result, err := d.Discover(context.Background(), "foo.example.com")
	assert.NoError(t, err)
	assert.Equal(t, "arn:wildcard", result)
}

func TestDiscover_ExactOverWildcard(t *testing.T) {
	now := time.Now()
	exact := &acm.CertificateSummary{
		CertificateArn:                     aws.String("arn:exact"),
		DomainName:                         aws.String("api.example.com"),
		SubjectAlternativeNameSummaries:     []*string{aws.String("api.example.com")},
		HasAdditionalSubjectAlternativeNames: aws.Bool(false),
		IssuedAt:                           &now,
	}
	wildcard := &acm.CertificateSummary{
		CertificateArn:                     aws.String("arn:wildcard"),
		DomainName:                         aws.String("*.example.com"),
		SubjectAlternativeNameSummaries:     []*string{aws.String("*.example.com")},
		HasAdditionalSubjectAlternativeNames: aws.Bool(false),
		IssuedAt:                           &now,
	}
	fake := &fakeACM{certs: []*acm.CertificateSummary{wildcard, exact}}
	d := NewCertificateDiscovery(fake)

	result, err := d.Discover(context.Background(), "api.example.com")
	assert.NoError(t, err)
	assert.Equal(t, "arn:exact", result)
}

func TestDiscover_NewerExactWins(t *testing.T) {
	older := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	certOld := &acm.CertificateSummary{
		CertificateArn:                     aws.String("arn:old"),
		DomainName:                         aws.String("api.example.com"),
		SubjectAlternativeNameSummaries:     []*string{aws.String("api.example.com")},
		HasAdditionalSubjectAlternativeNames: aws.Bool(false),
		IssuedAt:                           &older,
	}
	certNew := &acm.CertificateSummary{
		CertificateArn:                     aws.String("arn:new"),
		DomainName:                         aws.String("api.example.com"),
		SubjectAlternativeNameSummaries:     []*string{aws.String("api.example.com")},
		HasAdditionalSubjectAlternativeNames: aws.Bool(false),
		IssuedAt:                           &newer,
	}
	fake := &fakeACM{certs: []*acm.CertificateSummary{certOld, certNew}}
	d := NewCertificateDiscovery(fake)

	result, err := d.Discover(context.Background(), "api.example.com")
	assert.NoError(t, err)
	assert.Equal(t, "arn:new", result)
}

func TestDiscover_TruncatedSANs_FallsBackToDescribe(t *testing.T) {
	now := time.Now()
	cert := &acm.CertificateSummary{
		CertificateArn:                     aws.String("arn:truncated"),
		DomainName:                         aws.String("example.com"),
		SubjectAlternativeNameSummaries:     []*string{aws.String("example.com")}, // truncated, missing api.example.com
		HasAdditionalSubjectAlternativeNames: aws.Bool(true),
		IssuedAt:                           &now,
	}
	fake := &fakeACM{
		certs: []*acm.CertificateSummary{cert},
		describeCerts: map[string]*acm.CertificateDetail{
			"arn:truncated": {
				SubjectAlternativeNames: []*string{
					aws.String("example.com"),
					aws.String("api.example.com"),
				},
			},
		},
	}
	d := NewCertificateDiscovery(fake)

	result, err := d.Discover(context.Background(), "api.example.com")
	assert.NoError(t, err)
	assert.Equal(t, "arn:truncated", result)
	assert.Equal(t, 1, fake.descCallCount)

	// Second call uses cached domains — no additional DescribeCertificate call
	result2, err := d.Discover(context.Background(), "api.example.com")
	assert.NoError(t, err)
	assert.Equal(t, "arn:truncated", result2)
	assert.Equal(t, 1, fake.descCallCount, "expected DescribeCertificate result to be cached")
}

func TestDiscover_FallbackToDomainName(t *testing.T) {
	now := time.Now()
	cert := &acm.CertificateSummary{
		CertificateArn:                     aws.String("arn:fallback"),
		DomainName:                         aws.String("api.example.com"),
		SubjectAlternativeNameSummaries:     nil, // no SANs
		HasAdditionalSubjectAlternativeNames: aws.Bool(false),
		IssuedAt:                           &now,
	}
	fake := &fakeACM{certs: []*acm.CertificateSummary{cert}}
	d := NewCertificateDiscovery(fake)

	result, err := d.Discover(context.Background(), "api.example.com")
	assert.NoError(t, err)
	assert.Equal(t, "arn:fallback", result)
}

func TestDiscover_NoMatch(t *testing.T) {
	now := time.Now()
	cert := &acm.CertificateSummary{
		CertificateArn:                     aws.String("arn:other"),
		DomainName:                         aws.String("other.com"),
		SubjectAlternativeNameSummaries:     []*string{aws.String("other.com")},
		HasAdditionalSubjectAlternativeNames: aws.Bool(false),
		IssuedAt:                           &now,
	}
	fake := &fakeACM{certs: []*acm.CertificateSummary{cert}}
	d := NewCertificateDiscovery(fake)

	result, err := d.Discover(context.Background(), "api.example.com")
	assert.NoError(t, err)
	assert.Empty(t, result)
}

func TestDiscover_NonAccessDeniedError(t *testing.T) {
	fake := &fakeACM{listErr: fmt.Errorf("throttled")}
	d := NewCertificateDiscovery(fake)

	result, err := d.Discover(context.Background(), "api.example.com")
	assert.Error(t, err)
	assert.Empty(t, result)
}

func TestDiscover_ConcurrentCallsSingleListRequest(t *testing.T) {
	now := time.Now()
	cert := &acm.CertificateSummary{
		CertificateArn:                     aws.String("arn:cert"),
		DomainName:                         aws.String("api.example.com"),
		SubjectAlternativeNameSummaries:     []*string{aws.String("api.example.com")},
		HasAdditionalSubjectAlternativeNames: aws.Bool(false),
		IssuedAt:                           &now,
	}
	fake := &fakeACM{certs: []*acm.CertificateSummary{cert}}
	d := NewCertificateDiscovery(fake)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := d.Discover(context.Background(), "api.example.com")
			assert.NoError(t, err)
			assert.Equal(t, "arn:cert", result)
		}()
	}
	wg.Wait()

	// Mutex ensures only one ListCertificates call despite concurrent access
	assert.Equal(t, 1, fake.listCallCount)
}

func TestMatchesExact(t *testing.T) {
	tests := []struct {
		name     string
		cert     string
		hostname string
		want     bool
	}{
		{"exact match", "api.example.com", "api.example.com", true},
		{"case insensitive", "API.Example.COM", "api.example.com", true},
		{"different hostname", "api.example.com", "web.example.com", false},
		{"wildcard is not exact", "*.example.com", "api.example.com", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, matchesExact(tt.cert, tt.hostname))
		})
	}
}

func TestIsWildcardMatch(t *testing.T) {
	tests := []struct {
		name     string
		cert     string
		hostname string
		want     bool
	}{
		{"single sub-level matches", "*.example.com", "foo.example.com", true},
		{"case insensitive", "*.Example.COM", "foo.example.com", true},
		{"multi sub-level rejected", "*.example.com", "foo.bar.example.com", false},
		{"bare domain rejected", "*.example.com", "example.com", false},
		{"double wildcard rejected", "*.*.example.com", "foo.example.com", false},
		{"mid wildcard rejected", "foo.*.example.com", "foo.bar.example.com", false},
		{"exact domain not wildcard", "example.com", "example.com", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isWildcardMatch(tt.cert, tt.hostname))
		})
	}
}

func TestCertTimestamp(t *testing.T) {
	t1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2025, 2, 1, 0, 0, 0, 0, time.UTC)
	t3 := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		cert *acm.CertificateSummary
		want *time.Time
	}{
		{"all set returns IssuedAt", &acm.CertificateSummary{IssuedAt: &t1, ImportedAt: &t2, CreatedAt: &t3}, &t1},
		{"no IssuedAt returns ImportedAt", &acm.CertificateSummary{ImportedAt: &t2, CreatedAt: &t3}, &t2},
		{"only CreatedAt", &acm.CertificateSummary{CreatedAt: &t3}, &t3},
		{"all nil returns nil", &acm.CertificateSummary{}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := certTimestamp(tt.cert)
			if tt.want == nil {
				assert.Nil(t, got)
			} else {
				assert.Equal(t, *tt.want, *got)
			}
		})
	}
}
