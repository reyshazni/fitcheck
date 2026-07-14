package nodepool

import (
	"context"
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/reyshazni/fitcheck/internal/diagnosis"
)

// Collector groups cluster nodes into nodepools by label.
type Collector struct{}

type nodepoolGroup struct {
	name  string
	nodes []diagnosis.NodeInfo
}

// Collect lists all nodes and groups them by the given label key.
// Nodes without the label are skipped. The nameLabelKey is used to
// resolve a human-readable name for each nodepool.
func (c Collector) Collect(
	ctx context.Context,
	cl client.Client,
	labelKey string,
	nameLabelKey string,
) ([]diagnosis.NodepoolInfo, error) {
	var nodeList corev1.NodeList
	if err := cl.List(ctx, &nodeList); err != nil {
		return nil, fmt.Errorf("listing nodes: %w", err)
	}

	groups := groupNodesByLabel(nodeList.Items, labelKey, nameLabelKey)

	return sortedNodepools(groups), nil
}

func groupNodesByLabel(
	nodes []corev1.Node,
	labelKey string,
	nameLabelKey string,
) map[string]*nodepoolGroup {
	groups := make(map[string]*nodepoolGroup)

	for i := range nodes {
		poolID, ok := nodes[i].Labels[labelKey]
		if !ok {
			continue
		}

		group, exists := groups[poolID]
		if !exists {
			group = &nodepoolGroup{name: resolveName(nodes[i].Labels, nameLabelKey, poolID)}
			groups[poolID] = group
		}

		group.nodes = append(group.nodes, buildNodeInfo(nodes[i]))
	}

	return groups
}

func resolveName(labels map[string]string, nameLabelKey, fallback string) string {
	if name, ok := labels[nameLabelKey]; ok && name != "" {
		return name
	}

	return fallback
}

func buildNodeInfo(node corev1.Node) diagnosis.NodeInfo {
	return diagnosis.NodeInfo{
		Name:              node.Name,
		Labels:            node.Labels,
		Taints:            node.Spec.Taints,
		Allocatable:       node.Status.Allocatable,
		CreationTimestamp: node.CreationTimestamp.Time,
	}
}

func sortedNodepools(groups map[string]*nodepoolGroup) []diagnosis.NodepoolInfo {
	pools := make([]diagnosis.NodepoolInfo, 0, len(groups))

	for id, group := range groups {
		pools = append(pools, diagnosis.NodepoolInfo{
			ID:    id,
			Name:  group.name,
			Nodes: group.nodes,
		})
	}

	sort.Slice(pools, func(i, j int) bool {
		return pools[i].ID < pools[j].ID
	})

	return pools
}
