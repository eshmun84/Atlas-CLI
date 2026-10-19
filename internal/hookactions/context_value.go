package hookactions

import "context"

type projectCtxKey struct{}

// WithProject attaches a resolved project identity for maintenance Actions.
// hooksengine.Engine sets this before dispatch so Actions honor Env.Root/Home
// instead of re-resolving from process cwd alone.
func WithProject(ctx context.Context, ref ProjectRef) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, projectCtxKey{}, ref)
}

// ProjectFromContext returns a project attached by WithProject.
func ProjectFromContext(ctx context.Context) (ProjectRef, bool) {
	if ctx == nil {
		return ProjectRef{}, false
	}
	ref, ok := ctx.Value(projectCtxKey{}).(ProjectRef)
	return ref, ok
}

func resolveWithContext(ctx context.Context, override ResolveFunc) (ProjectRef, error) {
	if override != nil {
		return override()
	}
	if ref, ok := ProjectFromContext(ctx); ok {
		return ref, nil
	}
	return DefaultResolve()
}
