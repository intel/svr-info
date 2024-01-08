/*
 * Copyright (C) 2023 Intel Corporation
 * SPDX-License-Identifier: MIT
 */
package target

import (
	"testing"
)

func TestNew(t *testing.T) {
	localTarget := NewLocalTarget("hostname", "sudo")
	if localTarget == nil {
		t.Fatal("failed to create a local target")
	}
	remoteTarget := NewRemoteTarget("label", "hostname", "22", "user", "key", "pass", "sshpass", "sudo")
	if remoteTarget == nil {
		t.Fatal("failed to create a remote target")
	}
}
