package main

import "testing"

func TestSourceAddressedCommandSyntheticCorpus(t *testing.T) {
	testGroundedCommandSyntheticCorpus(t, "source-addressed-v9")
}

func TestSourceAddressedCommandFailureGates(t *testing.T) {
	testGroundedCommandFailureGates(t, "source-addressed-v9")
}

func TestSourceAddressedFlagsAndContract(t *testing.T) {
	testGroundedFlagsAndContract(t, "source-addressed-v9")
}

func TestSourceAddressedCommandNative(t *testing.T) {
	testPartitionedCandidateCommandNative(t, "source-addressed-v9")
}
