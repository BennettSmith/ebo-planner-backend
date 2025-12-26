package httpapi

import "context"

type subjectKey struct{}

func WithSubject(ctx context.Context, subjectID string) context.Context {
	return context.WithValue(ctx, subjectKey{}, subjectID)
}

func SubjectFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(subjectKey{}).(string)
	return v, ok && v != ""
}
