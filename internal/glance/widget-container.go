package glance

import (
	"context"
	"time"
)

type widgetContainer interface {
	childWidgets() widgets
}

type containerWidgetBase struct {
	Widgets widgets `yaml:"widgets"`
}

func (widget *containerWidgetBase) childWidgets() widgets {
	return widget.Widgets
}

func (widget *containerWidgetBase) _initializeWidgets() error {
	for i := range widget.Widgets {
		if err := widget.Widgets[i].initialize(); err != nil {
			return formatWidgetInitError(err, widget.Widgets[i])
		}
	}

	return nil
}

func (container *containerWidgetBase) _update(ctx context.Context) {
	now := time.Now()
	task := func(child widget) (struct{}, error) {
		refreshWidgetIfNeeded(ctx, child, &now)
		return struct{}{}, nil
	}

	_, _, _ = workerPoolDo(
		newJob(task, []widget(container.Widgets)).withWorkers(widgetNestedConcurrency).withContext(ctx),
	)
}

func (widget *containerWidgetBase) _setProviders(providers *widgetProviders) {
	for i := range widget.Widgets {
		widget.Widgets[i].setProviders(providers)
	}
}

func (widget *containerWidgetBase) _requiresUpdate(now *time.Time) bool {
	for i := range widget.Widgets {
		if widget.Widgets[i].requiresUpdate(now) {
			return true
		}
	}

	return false
}
