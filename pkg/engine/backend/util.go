package backend

import (
	"fmt"

	"kusionstack.io/kusion/pkg/apis/core/v1"
	"kusionstack.io/kusion/pkg/engine/states"
	"kusionstack.io/kusion/pkg/workspace"
)

// NewStateStorage news a StateStorage by configs of workspace, cli backend options, and environment variables.
func NewStateStorage(stack *v1.Stack, opts *BackendOptions) (states.StateStorage, error) {
	var backendConfigs *v1.BackendConfigs
	wsOperator, err := workspace.NewValidDefaultOperator()
	if err != nil {
		return nil, fmt.Errorf("new default workspace opearator failed, %w", err)
	}
	if wsOperator.WorkspaceExist(stack.Name) {
		var ws *v1.Workspace
		ws, err = wsOperator.GetWorkspace(stack.Name)
		if err != nil {
			return nil, fmt.Errorf("get config of workspace %s failed, %w", stack.Name, err)
		}
		backendConfigs = ws.Backends
		if backendConfigs != nil {
			if err = workspace.ValidateBackendConfigs(backendConfigs); err != nil {
				return nil, fmt.Errorf("invalid backend configs of workspace %s, %w", stack.Name, err)
			}
		}
	}
	stateStorageConfig, err := NewConfig(stack.Path, backendConfigs, opts)
	if err != nil {
		return nil, err
	}
	return stateStorageConfig.NewStateStorage()
}
