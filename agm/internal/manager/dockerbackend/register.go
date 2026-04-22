package dockerbackend

import "github.com/vbonnet/dear-agent/agm/internal/manager"

func init() {
	_ = manager.DefaultRegistry.Register("docker", func() (manager.Backend, error) {
		client := NewExecClient()
		return New(client, DefaultConfig()), nil
	})
}
