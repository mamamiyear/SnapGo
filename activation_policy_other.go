//go:build !darwin

package main

// installActivationPolicyHooks is only meaningful on macOS.
func installActivationPolicyHooks() {}

// showDockIcon is only meaningful on macOS.
func showDockIcon(_ bool) {}

// hideDockIcon is only meaningful on macOS.
func hideDockIcon() {}
