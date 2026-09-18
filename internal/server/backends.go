package server

//nolint:revive // blank imports register the zero-config backends
import (
	_ "github.com/acumen-ai-org/robotdreams/pkg/messaging/embedded"
	_ "github.com/acumen-ai-org/robotdreams/pkg/storage/localfs"
)
