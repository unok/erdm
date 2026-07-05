package server

import "testing"

func TestListenBaseURL_IPv4(t *testing.T) {
	got := listenBaseURL("127.0.0.1", 8080)
	want := "http://127.0.0.1:8080"
	if got != want {
		t.Fatalf("listenBaseURL: got %q, want %q", got, want)
	}
}

func TestListenBaseURL_IPv6(t *testing.T) {
	got := listenBaseURL("::1", 3123)
	want := "http://[::1]:3123"
	if got != want {
		t.Fatalf("listenBaseURL: got %q, want %q", got, want)
	}
}

func TestListenBaseURL_AllInterfaces(t *testing.T) {
	got := listenBaseURL("0.0.0.0", 3123)
	want := "http://0.0.0.0:3123"
	if got != want {
		t.Fatalf("listenBaseURL: got %q, want %q", got, want)
	}
}
