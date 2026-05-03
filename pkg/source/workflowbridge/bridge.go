// Package workflowbridge adapts a pkg/source.Adapter to pkg/workflow's
// SourceIndexer surface. Living here (rather than in pkg/workflow)
// keeps the dependency graph one-way: pkg/workflow knows nothing about
// pkg/source, and callers that want both wire them together via this
// thin shim.
//
// Usage:
//
//	a, err := sqlitesource.Open("sources.db")
//	r.OutputWriter.SourceIndexer = workflowbridge.New(a)
package workflowbridge

import (
	"context"

	"github.com/vbonnet/dear-agent/pkg/source"
	"github.com/vbonnet/dear-agent/pkg/workflow"
)

// New returns a SourceIndexer that forwards to a.Add. The Adapter
// must be non-nil; the bridge does not retain ownership and does not
// close the adapter.
func New(a source.Adapter) workflow.SourceIndexer {
	if a == nil {
		return nil
	}
	return &bridge{a: a}
}

type bridge struct{ a source.Adapter }

func (b *bridge) Name() string { return b.a.Name() }

func (b *bridge) Add(ctx context.Context, art workflow.SourceArtifact) error {
	src := source.Source{
		URI:       art.URI,
		Title:     art.Title,
		Snippet:   art.Snippet,
		Content:   art.Content,
		IndexedAt: art.IndexedAt,
		Metadata: source.Metadata{
			Cues:     art.Cues,
			WorkItem: art.WorkItem,
			Source:   "workflow",
			Custom: map[string]any{
				"content_type": art.ContentType,
			},
		},
	}
	_, err := b.a.Add(ctx, src)
	return err
}
