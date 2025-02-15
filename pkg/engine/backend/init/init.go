package init

import (
	"kusionstack.io/kusion/pkg/apis/core/v1"
	"kusionstack.io/kusion/pkg/engine/states"
	"kusionstack.io/kusion/pkg/engine/states/local"
	"kusionstack.io/kusion/pkg/engine/states/remote/http"
	"kusionstack.io/kusion/pkg/engine/states/remote/mysql"
	"kusionstack.io/kusion/pkg/engine/states/remote/oss"
	"kusionstack.io/kusion/pkg/engine/states/remote/s3"
)

// backends store all available backend
var backends map[string]func() states.Backend

// init backends map with all support backend
func init() {
	backends = map[string]func() states.Backend{
		v1.BackendLocal: local.NewLocalBackend,
		v1.BackendMysql: mysql.NewMysqlBackend,
		v1.BackendOss:   oss.NewOssBackend,
		v1.BackendS3:    s3.NewS3Backend,
		"http":          http.NewHTTPBackend,
	}
}

// GetBackend return backend, or nil if not exists
func GetBackend(name string) func() states.Backend {
	return backends[name]
}
