package diagnosis

import "fmt"

// CheckNodeSelector checks whether node labels satisfy the pod's nodeSelector.
// Returns nil if all selectors match, or a Rejection for the first mismatch.
func CheckNodeSelector(nodeSelector, nodeLabels map[string]string) *Rejection {
	for key, val := range nodeSelector {
		nodeVal, ok := nodeLabels[key]
		if !ok {
			return &Rejection{
				Category: CategoryNodeSelector,
				Reason:   fmt.Sprintf("nodeSelector %s=%s not matched, label missing", key, val),
			}
		}

		if nodeVal != val {
			return &Rejection{
				Category: CategoryNodeSelector,
				Reason:   fmt.Sprintf("nodeSelector %s=%s not matched, node has %s=%s", key, val, key, nodeVal),
			}
		}
	}

	return nil
}
