package imagepolicy

import (
	"sort"
	"strconv"
	"strings"
	"time"

	k8sfinding "k8s-aiops.local/backend/internal/finding"
)

// Evaluate runs the read-only image supply-chain / reproducibility analysis
// over the supplied observation bundle and returns an aggregated Status. It is
// pure: it never contacts a registry, never pulls a manifest and never mutates
// anything.
//
// Only the usages that are supplied are checked; an empty bundle is skipped
// (neither pass nor fail).
func Evaluate(clusterID int64, in Inputs, observedAt time.Time) Status {
	at := observedAt.UTC()
	status := Status{
		ClusterID:       clusterID,
		EvaluatedAt:     at,
		ImagesTotal:     0,
		ContainersTotal: len(in.Usages),
		BySeverity:      map[string]int{},
		ByFamily:        map[string]int{},
		Findings:        []Finding{},
	}

	// Group by the canonical image reference (repository + tag + digest) and
	// record every container that uses it.
	type entry struct {
		img        ImageInfo
		containers []ContainerRef
		namespaces map[string]bool
	}
	byImage := make(map[string]*entry, len(in.Usages))
	// repo -> set of tags, for the cross-tag skew check.
	repotags := make(map[string]map[string]bool)

	for _, u := range in.Usages {
		img := u.Image
		key := img.Repository + "|" + img.Tag + "|" + img.Digest
		e, ok := byImage[key]
		if !ok {
			e = &entry{img: img, namespaces: map[string]bool{}}
			byImage[key] = e
		}
		e.containers = append(e.containers, u.Container)
		e.namespaces[u.Container.Namespace] = true
		if img.Tag != "" {
			if repotags[img.Repository] == nil {
				repotags[img.Repository] = map[string]bool{}
			}
			repotags[img.Repository][img.Tag] = true
		}
	}

	status.ImagesTotal = len(byImage)
	imageOrder := make([]string, 0, len(byImage))
	for k := range byImage {
		imageOrder = append(imageOrder, k)
	}
	sort.Strings(imageOrder)

	for _, k := range imageOrder {
		e := byImage[k]
		img := e.img
		nsCount := len(e.namespaces)
		status.Total++

		// A digest pin makes the reference fully reproducible regardless of the
		// tag, so it is never reported as mutable or unpinned.
		pinned := img.Digest != ""
		mutable := !pinned && (img.Tag == "" || img.Tag == "latest")
		if mutable {
			status.MutableTagImages++
			status = status.withFinding(Finding{
				Code:     CodeMutableTag,
				Severity: SeverityWarning,
				Summary:  "镜像 " + imageRef(img) + " 使用可变 tag（:latest 或缺省），重新部署可能静默换成不同构建",
				Resource: k8sfinding.ResourceCitation{Kind: "Image", Name: img.Repository},
				Details: map[string]string{
					"family":      FamilySupplyChain,
					"tag":         defaultString(img.Tag, "latest"),
					"containers":  strconv.Itoa(len(e.containers)),
					"rationale":   "缺省 tag 在 Kubernetes 中等同于 :latest；镜像仓库一旦重新 push 同名 tag，线上运行的内容会随之改变，CVE 修复也可能无法真正落地。",
					"remediation": "改用固定 tag 或 digest 钉住（推荐 @sha256:...），并通过 CI 在镜像变更时自动升级引用。",
				},
				ObservedAt: k8sfinding.RFC3339(at),
			})
		} else if !pinned {
			status.UnpinnedImages++
			status = status.withFinding(Finding{
				Code:     CodeNoDigestPin,
				Severity: SeverityInfo,
				Summary:  "镜像 " + imageRef(img) + " 仅按 tag 引用，未钉 digest，存在被重新指向的风险",
				Resource: k8sfinding.ResourceCitation{Kind: "Image", Name: img.Repository},
				Details: map[string]string{
					"family":      FamilySupplyChain,
					"tag":         img.Tag,
					"containers":  strconv.Itoa(len(e.containers)),
					"rationale":   "即使 tag 固定，镜像仓库仍可能把该 tag 重新指向不同 manifest，导致不可复现的部署。",
					"remediation": "在 tag 之外追加 @sha256:... digest 钉住具体 manifest。",
				},
				ObservedAt: k8sfinding.RFC3339(at),
			})
		}

		if img.PullPolicy == "Always" && mutable {
			status.Total++
			status = status.withFinding(Finding{
				Code:     CodePullAlwaysLatest,
				Severity: SeverityInfo,
				Summary:  "镜像 " + imageRef(img) + " 使用 imagePullPolicy: Always 且为可变 tag，每次重启都会重新拉取当前 :latest",
				Resource: k8sfinding.ResourceCitation{Kind: "Image", Name: img.Repository},
				Details: map[string]string{
					"family":      FamilySupplyChain,
					"pull_policy": img.PullPolicy,
					"tag":         defaultString(img.Tag, "latest"),
					"rationale":   "Always + 可变 tag 意味着节点重启或驱逐后会拉到最新的 :latest，难以回滚到确定版本。",
					"remediation": "钉死 tag/digest 后将 imagePullPolicy 改为 IfNotPresent，或显式使用 digest。",
				},
				ObservedAt: k8sfinding.RFC3339(at),
			})
		}

		if nsCount > 1 {
			status.Total++
			status = status.withFinding(Finding{
				Code:     CodeSharedAcrossNamespaces,
				Severity: SeverityInfo,
				Summary:  "镜像 " + img.Repository + " 在 " + strconv.Itoa(nsCount) + " 个命名空间中运行，爆炸半径与灰度复杂度上升",
				Resource: k8sfinding.ResourceCitation{Kind: "Image", Name: img.Repository},
				Details: map[string]string{
					"family":      FamilySupplyChain,
					"namespaces":  strconv.Itoa(nsCount),
					"containers":  strconv.Itoa(len(e.containers)),
					"rationale":   "同一镜像跨命名空间部署时，一处变更会影响多处，且各命名空间可能停留在不同版本。",
					"remediation": "统一版本来源，按命名空间灰度，或纳入镜像仓库的不可变 tag 策略。",
				},
				ObservedAt: k8sfinding.RFC3339(at),
			})
		}
	}

	// Cross-tag skew: one repository referenced by several tags.
	repoOrder := make([]string, 0, len(repotags))
	for r := range repotags {
		repoOrder = append(repoOrder, r)
	}
	sort.Strings(repoOrder)
	for _, repo := range repoOrder {
		tags := repotags[repo]
		if len(tags) <= 1 {
			continue
		}
		status.Total++
		status = status.withFinding(Finding{
			Code:     CodeMultipleTags,
			Severity: SeverityInfo,
			Summary:  "镜像仓库 " + repo + " 被 " + strconv.Itoa(len(tags)) + " 个不同的 tag 引用，存在版本漂移",
			Resource: k8sfinding.ResourceCitation{Kind: "Image", Name: repo},
			Details: map[string]string{
				"family":      FamilySupplyChain,
				"tags":        strings.Join(tagList(tags), ","),
				"rationale":   "同一仓库用多个 tag 会让人误以为各工作负载版本一致，实际可能相差很远，出问题难定位。",
				"remediation": "收敛为单一不可变 tag 或 digest，淘汰冗余 tag。",
			},
			ObservedAt: k8sfinding.RFC3339(at),
		})
	}

	status.Passed = status.Total - status.Failed
	sortFindings(status.Findings)
	return status
}

// --- helpers ---------------------------------------------------------------

func (s Status) withFinding(f Finding) Status {
	s.Findings = append(s.Findings, f)
	s.Failed++
	if s.BySeverity == nil {
		s.BySeverity = map[string]int{}
	}
	s.BySeverity[f.Severity]++
	if s.ByFamily == nil {
		s.ByFamily = map[string]int{}
	}
	if family := f.Details["family"]; family != "" {
		s.ByFamily[family]++
	}
	return s
}

var severityRank = map[string]int{SeverityCritical: 0, SeverityWarning: 1, SeverityInfo: 2}

func sortFindings(findings []Finding) {
	sort.SliceStable(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if ra, rb := severityRank[a.Severity], severityRank[b.Severity]; ra != rb {
			return ra < rb
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		if a.Resource.Name != b.Resource.Name {
			return a.Resource.Name < b.Resource.Name
		}
		return a.Resource.Namespace < b.Resource.Namespace
	})
}

// ParseImage decomposes a container image reference into repository, tag and
// digest. A registry port colon (registry.io:5000/...) is not mistaken for a
// tag because the tag colon must come after the last slash.
//
// It is exported so the M65 collector can turn the raw image strings it reads
// from the API server into the structured form Evaluate expects.
func ParseImage(ref string) ImageInfo {
	digest := ""
	if i := strings.LastIndex(ref, "@"); i >= 0 {
		digest = ref[i+1:]
		ref = ref[:i]
	}
	tag := ""
	if i := strings.LastIndex(ref, ":"); i >= 0 {
		if slash := strings.LastIndex(ref, "/"); slash < 0 || i > slash {
			tag = ref[i+1:]
			ref = ref[:i]
		}
	}
	return ImageInfo{Repository: ref, Tag: tag, Digest: digest}
}

func imageRef(img ImageInfo) string {
	if img.Digest != "" {
		return img.Repository + ":" + defaultString(img.Tag, "latest") + "@" + img.Digest
	}
	if img.Tag != "" {
		return img.Repository + ":" + img.Tag
	}
	return img.Repository + ":latest"
}

func defaultString(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func tagList(tags map[string]bool) []string {
	out := make([]string, 0, len(tags))
	for t := range tags {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}
