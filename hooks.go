package tributary

import (
	"context"

	"github.com/nilpntr/tributary/tributaryhook"
)

// Re-export hook types from tributaryhook for backward compatibility
type Hook = tributaryhook.Hook
type BaseHook = tributaryhook.BaseHook

// Re-export encryption types
type Encryptor = tributaryhook.Encryptor
type EncryptHook = tributaryhook.EncryptHook
type SecretboxEncryptor = tributaryhook.SecretboxEncryptor

// Re-export hook functions
var (
	NewEncryptHook        = tributaryhook.NewEncryptHook
	NewSecretboxEncryptor = tributaryhook.NewSecretboxEncryptor
	RunBeforeInsertHooks  = tributaryhook.RunBeforeInsertHooks
	RunAfterInsertHooks   = tributaryhook.RunAfterInsertHooks
	RunBeforeWorkHooks    = tributaryhook.RunBeforeWorkHooks
	RunAfterWorkHooks     = tributaryhook.RunAfterWorkHooks
)

// Keep the old hook runner function names for backward compatibility
func runBeforeInsertHooks(ctx context.Context, hooks []Hook, args StepArgs, argsBytes []byte) ([]byte, error) {
	return RunBeforeInsertHooks(ctx, hooks, args, argsBytes)
}

func runAfterInsertHooks(ctx context.Context, hooks []Hook, stepID int64, args StepArgs) {
	RunAfterInsertHooks(ctx, hooks, stepID, args)
}

func runBeforeWorkHooks(ctx context.Context, hooks []Hook, stepID int64, kind string, argsBytes []byte) ([]byte, error) {
	return RunBeforeWorkHooks(ctx, hooks, stepID, kind, argsBytes)
}

func runAfterWorkHooks(ctx context.Context, hooks []Hook, stepID int64, result []byte, err error) ([]byte, error) {
	return RunAfterWorkHooks(ctx, hooks, stepID, result, err)
}
