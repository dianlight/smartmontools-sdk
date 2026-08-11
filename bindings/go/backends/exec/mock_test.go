// SPDX-License-Identifier: GPL-2.0-or-later

package exec

import (
	"context"
	"errors"
	osexec "os/exec"
)

// mockCmd implements Cmd for tests.
type mockCmd struct {
	osexec.Cmd
	output []byte
	err    error
}

func (m *mockCmd) Output() ([]byte, error) {
	return m.output, m.err
}

func (m *mockCmd) Run() error {
	return m.err
}

func (m *mockCmd) CombinedOutput() ([]byte, error) {
	return m.output, m.err
}

// mockCommander implements Commander for tests.
type mockCommander struct {
	cmds map[string]*mockCmd
}

func (m *mockCommander) Command(ctx context.Context, logger LogAdapter, name string, arg ...string) Cmd {
	key := name
	for _, a := range arg {
		key += " " + a
	}
	if cmd, ok := m.cmds[key]; ok {
		return cmd
	}
	return &mockCmd{err: errors.New("mock command not configured")}
}
