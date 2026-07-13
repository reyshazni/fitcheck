package diagnosis

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
)

// Verdict represents the scheduling fit result for a nodepool.
type Verdict string

// RejectionCategory classifies why a nodepool rejected a pod.
type RejectionCategory int

// Rejection holds the reason a nodepool cannot schedule a pod.
type Rejection struct {
	Category RejectionCategory
	Reason   string
}

// NodeInfo holds the scheduling-relevant properties of a single node.
type NodeInfo struct {
	Name        string
	Labels      map[string]string
	Taints      []corev1.Taint
	Allocatable corev1.ResourceList
}

// NodepoolInfo groups nodes belonging to the same nodepool.
type NodepoolInfo struct {
	ID    string
	Name  string
	Nodes []NodeInfo
}

// NodepoolDiagnosis is the result of checking one nodepool against a pod.
type NodepoolDiagnosis struct {
	NodepoolID   string
	NodepoolName string
	Verdict      Verdict
	Rejection    *Rejection
	FittingNodes int
	TotalNodes   int
}

const (
	Accepted  Verdict = "Accepted"
	Rejected  Verdict = "Rejected"
	Candidate Verdict = "Candidate"
	NoStock   Verdict = "NoStock"
)

const (
	CategoryTaint        RejectionCategory = 1
	CategoryNodeSelector RejectionCategory = 2
	CategoryAffinity     RejectionCategory = 3
	CategoryResources    RejectionCategory = 4
)

// EventType returns the Kubernetes event type for this diagnosis.
func (d NodepoolDiagnosis) EventType() string {
	if d.Verdict == Accepted {
		return corev1.EventTypeNormal
	}

	return corev1.EventTypeWarning
}

// EventReason returns the Kubernetes event reason string for this diagnosis.
func (d NodepoolDiagnosis) EventReason() string {
	return "Nodepool" + string(d.Verdict)
}

// Message returns a human-readable diagnosis message.
func (d NodepoolDiagnosis) Message() string {
	if d.Verdict == Accepted {
		return fmt.Sprintf("nodepool/%s: fits %d/%d nodes", d.NodepoolName, d.FittingNodes, d.TotalNodes)
	}

	return fmt.Sprintf("nodepool/%s: %s", d.NodepoolName, d.Rejection.Reason)
}
