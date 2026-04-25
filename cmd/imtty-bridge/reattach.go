package main

import (
	"context"
	"strings"

	"imtty/internal/session"
)

type sessionLister interface {
	ListSessions(ctx context.Context) ([]string, error)
}

func restoreDetachedSessions(ctx context.Context, registry *session.Registry, lister sessionLister, prefix string) ([]session.View, error) {
	sessionNames, err := lister.ListSessions(ctx)
	if err != nil {
		return nil, err
	}

	restored := make([]session.View, 0, len(sessionNames))
	for _, sessionName := range sessionNames {
		project := strings.TrimPrefix(sessionName, prefix)
		if project == "" || project == sessionName {
			continue
		}

		view, err := registry.Reattach(project)
		if err != nil {
			continue
		}
		restored = append(restored, view)
	}

	return restored, nil
}
