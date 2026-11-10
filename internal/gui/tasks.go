package gui

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

const taskLogLimit = 1024 * 1024

type Task struct {
	ID            string    `json:"id"`
	Kind          string    `json:"kind"`
	Status        string    `json:"status"`
	Output        string    `json:"output"`
	Error         string    `json:"error,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	StopRequested bool      `json:"stop_requested,omitempty"`
}

type taskLog struct {
	mutex  sync.Mutex
	buffer []byte
}

func (l *taskLog) Write(p []byte) (int, error) {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	remaining := taskLogLimit - len(l.buffer)
	if remaining <= 0 {
		return len(p), nil
	}
	if len(p) > remaining {
		l.buffer = append(l.buffer, p[:remaining]...)
		l.buffer = append(l.buffer, []byte("\n[output truncated]\n")...)
		return len(p), nil
	}
	l.buffer = append(l.buffer, p...)
	return len(p), nil
}

func (l *taskLog) String() string {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return string(l.buffer)
}

type guiTask struct {
	mutex         sync.Mutex
	id            string
	kind          string
	status        string
	errorMessage  string
	createdAt     time.Time
	updatedAt     time.Time
	stopRequested bool
	log           taskLog
	cancel        context.CancelFunc
}

func (t *guiTask) snapshot() Task {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	return Task{
		ID: t.id, Kind: t.kind, Status: t.status, Output: t.log.String(),
		Error: t.errorMessage, CreatedAt: t.createdAt, UpdatedAt: t.updatedAt,
		StopRequested: t.stopRequested,
	}
}

func (t *guiTask) finish(err error) {
	t.mutex.Lock()
	defer t.mutex.Unlock()
	t.updatedAt = time.Now()
	switch {
	case err == nil:
		t.status = "succeeded"
		t.errorMessage = ""
	case t.stopRequested || errors.Is(err, context.Canceled):
		t.status = "canceled"
		t.errorMessage = err.Error()
	default:
		t.status = "failed"
		t.errorMessage = err.Error()
	}
}

type taskManager struct {
	mutex sync.Mutex
	tasks map[string]*guiTask
}

func newTaskManager() *taskManager {
	return &taskManager{tasks: map[string]*guiTask{}}
}

func newTaskID() (string, error) {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return "task-" + hex.EncodeToString(b[:]), nil
}

func (m *taskManager) start(kind string, action func(context.Context, io.Writer) error) (Task, error) {
	id, err := newTaskID()
	if err != nil {
		return Task{}, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	task := &guiTask{
		id: id, kind: kind, status: "running", createdAt: time.Now(),
		updatedAt: time.Now(), cancel: cancel,
	}
	m.mutex.Lock()
	m.tasks[id] = task
	m.mutex.Unlock()

	go func() {
		err := action(ctx, &task.log)
		if err == nil {
			_, _ = fmt.Fprintf(&task.log, "\nTask completed successfully.\n")
		}
		task.finish(err)
	}()
	return task.snapshot(), nil
}

func (m *taskManager) get(id string) (Task, bool) {
	m.mutex.Lock()
	task, ok := m.tasks[id]
	m.mutex.Unlock()
	if !ok {
		return Task{}, false
	}
	return task.snapshot(), true
}

func (m *taskManager) stop(id string) (Task, bool) {
	m.mutex.Lock()
	task, ok := m.tasks[id]
	m.mutex.Unlock()
	if !ok {
		return Task{}, false
	}
	task.mutex.Lock()
	if task.status == "running" {
		task.stopRequested = true
		task.updatedAt = time.Now()
		task.cancel()
	}
	task.mutex.Unlock()
	return task.snapshot(), true
}
