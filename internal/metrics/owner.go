package metrics

import (
	"context"
	"log/slog"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ownerInfo struct {
	Kind string
	Name string
}

const kindReplicaSet = "ReplicaSet"

func resolveOwner(
	ctx context.Context,
	reader client.Reader,
	pod *corev1.Pod,
	cache map[types.UID]ownerInfo,
) ownerInfo {
	if len(pod.OwnerReferences) == 0 {
		return ownerInfo{}
	}

	ref := pod.OwnerReferences[0]

	if ref.Kind != kindReplicaSet {
		return ownerInfo{Kind: ref.Kind, Name: ref.Name}
	}

	if cached, ok := cache[ref.UID]; ok {
		return cached
	}

	info := resolveReplicaSet(ctx, reader, pod.Namespace, ref.Name)
	cache[ref.UID] = info

	return info
}

func resolveReplicaSet(
	ctx context.Context,
	reader client.Reader,
	namespace string,
	name string,
) ownerInfo {
	var rs appsv1.ReplicaSet

	key := types.NamespacedName{Namespace: namespace, Name: name}
	if err := reader.Get(ctx, key, &rs); err != nil {
		slog.Warn("failed to get ReplicaSet for owner resolution", "name", name, "error", err)

		return ownerInfo{Kind: kindReplicaSet, Name: name}
	}

	if len(rs.OwnerReferences) > 0 {
		return ownerInfo{Kind: rs.OwnerReferences[0].Kind, Name: rs.OwnerReferences[0].Name}
	}

	return ownerInfo{Kind: kindReplicaSet, Name: name}
}
