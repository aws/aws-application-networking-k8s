package lattice

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice"
	"github.com/aws/aws-sdk-go-v2/service/vpclattice/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	pkg_aws "github.com/aws/aws-application-networking-k8s/pkg/aws"
	mocks "github.com/aws/aws-application-networking-k8s/pkg/aws/services"
	model "github.com/aws/aws-application-networking-k8s/pkg/model/lattice"
	"github.com/aws/aws-application-networking-k8s/pkg/utils/gwlog"
)

// findSvcExportTGByAccountScan is the previous findSvcExportTG, which read the
// tags of every target group in the account through List on each lookup and
// returned the first match in list order. tgs is the output of List.
func findSvcExportTGByAccountScan(tgs []tgListOutput, svcImportTg model.SvcImportTargetGroup) (string, error) {
	for _, tg := range tgs {
		tgTags := model.TGTagFieldsFromTags(tg.tags)
		svcMatch := tgTags.IsSourceTypeServiceExport() && (tgTags.K8SServiceName == svcImportTg.K8SServiceName) &&
			(tgTags.K8SServiceNamespace == svcImportTg.K8SServiceNamespace)
		clusterMatch := (svcImportTg.K8SClusterName == "") || (tgTags.K8SClusterName == svcImportTg.K8SClusterName)
		vpcMatch := (svcImportTg.VpcId == "") || (svcImportTg.VpcId == aws.ToString(tg.tgSummary.VpcIdentifier))
		if svcMatch && clusterMatch && vpcMatch {
			return *tg.tgSummary.Id, nil
		}
	}
	return "", errors.New("target group for service import could not be found")
}

type svcImportAccount struct {
	tgs  []types.TargetGroupSummary
	tags map[string]map[string]string
}

func (a *svcImportAccount) add(id, name, vpc string, tags map[string]string) {
	arn := "arn:aws:vpc-lattice:us-west-2:123456789012:targetgroup/" + id
	a.tgs = append(a.tgs, types.TargetGroupSummary{
		Arn:           aws.String(arn),
		Id:            aws.String(id),
		Name:          aws.String(name),
		VpcIdentifier: aws.String(vpc),
	})
	if tags != nil {
		a.tags[arn] = tags
	}
}

// manager returns a target group manager backed by the account. The ARNs of
// every GetTagsForArns call are appended to requested, and only the tags of
// requested ARNs are returned.
func (a *svcImportAccount) manager(t *testing.T, requested *[][]string) *defaultTargetGroupManager {
	c := gomock.NewController(t)
	mockLattice := mocks.NewMockLattice(c)
	mockTagging := mocks.NewMockTagging(c)
	mockCloud := pkg_aws.NewMockCloud(c)
	mockCloud.EXPECT().Lattice().Return(mockLattice).AnyTimes()
	mockCloud.EXPECT().Tagging().Return(mockTagging).AnyTimes()
	mockLattice.EXPECT().ListTargetGroupsAsList(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in *vpclattice.ListTargetGroupsInput) ([]types.TargetGroupSummary, error) {
			assert.Nil(t, in.VpcIdentifier, "lookup must keep the account-wide list order")
			return a.tgs, nil
		}).AnyTimes()
	mockTagging.EXPECT().GetTagsForArns(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, arns []string) (map[string]map[string]string, error) {
			*requested = append(*requested, arns)
			out := map[string]map[string]string{}
			for _, arn := range arns {
				if tags, ok := a.tags[arn]; ok {
					out[arn] = tags
				}
			}
			return out, nil
		}).AnyTimes()
	return NewTargetGroupManager(gwlog.FallbackLogger, mockCloud, nil)
}

func generatedTgNameFor(svc, ns string) string {
	return model.GenerateTgName(model.TargetGroupSpec{TargetGroupTagFields: model.TargetGroupTagFields{
		K8SServiceName: svc, K8SServiceNamespace: ns}})
}

func svcExportTags(svc, ns, cluster string) map[string]string {
	return model.TagsFromTGTagFields(model.TargetGroupTagFields{K8SSourceType: model.SourceTypeSvcExport,
		K8SServiceName: svc, K8SServiceNamespace: ns, K8SClusterName: cluster})
}

func countArns(requested [][]string) int {
	n := 0
	for _, arns := range requested {
		n += len(arns)
	}
	return n
}

var (
	// "ns"/"a-svc" and "ns-a"/"svc" share the name prefix k8s-ns-a-svc-.
	svcImportNamespaces = []string{"default", "ns", "ns-a", "a", strings.Repeat("n", 60)}
	svcImportServices   = []string{"svc", "svc-a", "a-svc", "a", "foo",
		// Equal after truncation to model.MaxNameLength.
		strings.Repeat("s", 55) + "-one", strings.Repeat("s", 55) + "-two"}
	svcImportClusters = []string{"c1", "c2", "c3"}
	svcImportVPCs     = []string{"vpc-1", "vpc-2", "vpc-3"}
	svcImportSources  = []model.K8SSourceType{model.SourceTypeSvcExport, model.SourceTypeSvcExport,
		model.SourceTypeHTTPRoute, model.SourceTypeGRPCRoute, model.SourceTypeTLSRoute}
)

func pickOne[T any](r *rand.Rand, s []T) T { return s[r.Intn(len(s))] }

// newRandomSvcImportAccount returns n target groups, most named by the
// controller from their tags, some named by other tools, some with unrelated
// or missing tags.
func newRandomSvcImportAccount(r *rand.Rand, n int) *svcImportAccount {
	a := &svcImportAccount{tags: map[string]map[string]string{}}
	for i := 0; i < n; i++ {
		fields := model.TargetGroupTagFields{
			K8SClusterName:      pickOne(r, svcImportClusters),
			K8SSourceType:       pickOne(r, svcImportSources),
			K8SServiceName:      pickOne(r, svcImportServices),
			K8SServiceNamespace: pickOne(r, svcImportNamespaces),
		}
		name := model.GenerateTgName(model.TargetGroupSpec{TargetGroupTagFields: fields})
		tags := model.TagsFromTGTagFields(fields)
		switch r.Intn(20) {
		case 0:
			name = fmt.Sprintf("custom-tg-%d", i) // named outside the controller, controller tags
		case 1:
			name = fmt.Sprintf("k8s-imported-%d", i) // k8s- prefix, not a generated name
		case 2:
			tags = map[string]string{"team": "other"}
		case 3:
			tags = nil
		}
		a.add(fmt.Sprintf("tg-%05d", i), name, pickOne(r, svcImportVPCs), tags)
	}
	return a
}

// Every import resolves to the same target group, or the same miss, as the
// account-wide scan, while reading far fewer tags.
func Test_findSvcExportTG_SameSelectionAsAccountScan(t *testing.T) {
	ctx := context.Background()
	var imports []model.SvcImportTargetGroup
	for _, ns := range svcImportNamespaces {
		for _, svc := range append(svcImportServices, "missing") {
			for _, cluster := range append(svcImportClusters, "", "c-missing") {
				for _, vpc := range append(svcImportVPCs, "", "vpc-missing") {
					imports = append(imports, model.SvcImportTargetGroup{
						K8SServiceName: svc, K8SServiceNamespace: ns, K8SClusterName: cluster, VpcId: vpc,
					})
				}
			}
		}
	}

	found, missed, scanReads, reads := 0, 0, 0, 0
	for seed := int64(1); seed <= 5; seed++ {
		account := newRandomSvcImportAccount(rand.New(rand.NewSource(seed)), 300)
		var requested [][]string
		s := account.manager(t, &requested)
		listed, err := s.List(ctx)
		require.NoError(t, err)
		listReads := countArns(requested)
		for _, imp := range imports {
			wantId, wantErr := findSvcExportTGByAccountScan(listed, imp)
			scanReads += listReads

			requested = nil
			gotId, gotErr := s.findSvcExportTG(ctx, imp)
			reads += countArns(requested)

			require.Equal(t, wantId, gotId, "seed %d import %+v", seed, imp)
			require.Equal(t, wantErr, gotErr, "seed %d import %+v", seed, imp)
			if wantErr == nil {
				found++
			} else {
				missed++
			}
		}
	}
	t.Logf("%d found, %d missed, %d tags read (account-wide scan: %d)", found, missed, reads, scanReads)
	assert.Greater(t, found, 1000)
	assert.Greater(t, missed, 1000)
	assert.Less(t, reads*10, scanReads, "should read under a tenth of the tags the account-wide scan reads")
}

// Only target groups in the import's VPC that carry the imported service's
// generated name, or a name the controller does not generate, have their tags
// read. A lookup with no such target group reads no tags.
func Test_findSvcExportTG_ReadsTagsOnlyForCandidates(t *testing.T) {
	ctx := context.Background()
	longSvc, longNs := strings.Repeat("s", 60), strings.Repeat("n", 60)
	a := &svcImportAccount{tags: map[string]map[string]string{}}
	a.add("other-svc", generatedTgNameFor("other", "ns"), "vpc-1", svcExportTags("other", "ns", "c1"))
	// k8s-ns-checkout-api-... starts with k8s-ns-checkout-, so it is read and rejected by its tags.
	a.add("prefix-collision", generatedTgNameFor("checkout-api", "ns"), "vpc-1", svcExportTags("checkout-api", "ns", "c1"))
	a.add("other-vpc", generatedTgNameFor("checkout", "ns"), "vpc-2", svcExportTags("checkout", "ns", "c2"))
	a.add("custom", "custom-name", "vpc-1", svcExportTags("unrelated", "ns", "c1"))
	a.add("upper-suffix", "k8s-ns-svc-ABCDEFGHIJ", "vpc-1", nil)
	a.add("short-suffix", "k8s-ns-svc-abcdefghi", "vpc-1", nil)
	a.add("long", generatedTgNameFor(longSvc, longNs), "vpc-1", svcExportTags(longSvc, longNs, "c1"))
	a.add("want", generatedTgNameFor("checkout", "ns"), "vpc-1", svcExportTags("checkout", "ns", "c1"))
	a.add("duplicate", generatedTgNameFor("checkout", "ns"), "vpc-1", svcExportTags("checkout", "ns", "c1"))
	for i := 0; i < 50; i++ {
		svc := fmt.Sprintf("svc-%d", i)
		a.add("filler-"+svc, generatedTgNameFor(svc, "ns"), "vpc-1", svcExportTags(svc, "ns", "c1"))
	}
	arns := func(ids ...string) [][]string {
		out := make([]string, len(ids))
		for i, id := range ids {
			out[i] = "arn:aws:vpc-lattice:us-west-2:123456789012:targetgroup/" + id
		}
		return [][]string{out}
	}

	tests := []struct {
		name      string
		svcImport model.SvcImportTargetGroup
		wantId    string
		wantReads [][]string
	}{
		{
			name:      "cluster and VPC known",
			svcImport: model.SvcImportTargetGroup{K8SServiceName: "checkout", K8SServiceNamespace: "ns", K8SClusterName: "c1", VpcId: "vpc-1"},
			wantId:    "want",
			wantReads: arns("prefix-collision", "custom", "upper-suffix", "short-suffix", "want", "duplicate"),
		},
		{
			name:      "cluster and VPC unknown",
			svcImport: model.SvcImportTargetGroup{K8SServiceName: "checkout", K8SServiceNamespace: "ns"},
			wantId:    "other-vpc",
			wantReads: arns("prefix-collision", "other-vpc", "custom", "upper-suffix", "short-suffix", "want", "duplicate"),
		},
		{
			name:      "truncated name",
			svcImport: model.SvcImportTargetGroup{K8SServiceName: longSvc, K8SServiceNamespace: longNs, VpcId: "vpc-1"},
			wantId:    "long",
			wantReads: arns("custom", "upper-suffix", "short-suffix", "long"),
		},
		{
			name:      "export missing",
			svcImport: model.SvcImportTargetGroup{K8SServiceName: "missing", K8SServiceNamespace: "ns"},
			wantReads: arns("custom", "upper-suffix", "short-suffix"),
		},
		{
			name:      "no candidate in VPC",
			svcImport: model.SvcImportTargetGroup{K8SServiceName: "checkout", K8SServiceNamespace: "ns", VpcId: "vpc-3"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requested [][]string
			s := a.manager(t, &requested)
			id, err := s.findSvcExportTG(ctx, tt.svcImport)
			if tt.wantId == "" {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.wantId, id)
			assert.Equal(t, tt.wantReads, requested)
		})
	}
}

func Test_findSvcExportTG_Errors(t *testing.T) {
	ctx := context.Background()
	svcImport := model.SvcImportTargetGroup{K8SServiceName: "checkout", K8SServiceNamespace: "ns"}
	tg := types.TargetGroupSummary{Arn: aws.String("arn"), Id: aws.String("id"),
		Name: aws.String(generatedTgNameFor("checkout", "ns")), VpcIdentifier: aws.String("vpc-1")}

	newManager := func(t *testing.T) (*defaultTargetGroupManager, *mocks.MockLattice, *mocks.MockTagging) {
		c := gomock.NewController(t)
		mockLattice := mocks.NewMockLattice(c)
		mockTagging := mocks.NewMockTagging(c)
		mockCloud := pkg_aws.NewMockCloud(c)
		mockCloud.EXPECT().Lattice().Return(mockLattice).AnyTimes()
		mockCloud.EXPECT().Tagging().Return(mockTagging).AnyTimes()
		return NewTargetGroupManager(gwlog.FallbackLogger, mockCloud, nil), mockLattice, mockTagging
	}

	t.Run("list fails", func(t *testing.T) {
		s, mockLattice, _ := newManager(t)
		mockLattice.EXPECT().ListTargetGroupsAsList(ctx, gomock.Any()).Return(nil, errors.New("list failed"))
		_, err := s.findSvcExportTG(ctx, svcImport)
		assert.EqualError(t, err, "list failed")
	})
	t.Run("tag read fails", func(t *testing.T) {
		s, mockLattice, mockTagging := newManager(t)
		mockLattice.EXPECT().ListTargetGroupsAsList(ctx, gomock.Any()).Return([]types.TargetGroupSummary{tg}, nil)
		mockTagging.EXPECT().GetTagsForArns(ctx, []string{"arn"}).Return(nil, errors.New("tags failed"))
		_, err := s.findSvcExportTG(ctx, svcImport)
		assert.EqualError(t, err, "tags failed")
	})
}
