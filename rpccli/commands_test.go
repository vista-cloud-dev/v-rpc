package rpccli

import (
	"testing"

	"github.com/vista-cloud-dev/clikit"
)

// With docker transport, an engine container is the one irreducible input. When
// neither --container nor $M_<ENGINE>_CONTAINER is set, execer must fail with a
// USAGE-coded error so clikit answers it with the verb's help (rather than the
// driver later failing with a cryptic "no level in engine output").
func TestExecer_DockerRequiresContainer(t *testing.T) {
	t.Setenv("M_YDB_CONTAINER", "")
	e := engineConn{Engine: "ydb", Transport: "docker"}
	_, ferr := e.execer()
	if ferr == nil {
		t.Fatal("want a USAGE error when docker transport has no container, got nil")
	}
	if ferr.Exit != clikit.ExitUsage {
		t.Errorf("exit = %d, want ExitUsage(%d) — %s", ferr.Exit, clikit.ExitUsage, ferr.Message)
	}
}

// A container supplied via $M_<ENGINE>_CONTAINER satisfies the precondition (the
// container check must not fire); any later error must not be the USAGE one.
func TestExecer_ContainerFromEnvSatisfies(t *testing.T) {
	t.Setenv("M_YDB_CONTAINER", "vehu")
	e := engineConn{Engine: "ydb", Transport: "docker"}
	_, ferr := e.execer()
	if ferr != nil && ferr.Exit == clikit.ExitUsage {
		t.Errorf("container present via env but still got a USAGE error: %s", ferr.Message)
	}
}

// Remote transport needs no container (it connects by URL/namespace), so the
// container precondition must not fire for it.
func TestExecer_RemoteDoesNotRequireContainer(t *testing.T) {
	t.Setenv("M_IRIS_CONTAINER", "")
	e := engineConn{Engine: "iris", Transport: "remote"}
	_, ferr := e.execer()
	if ferr != nil && ferr.Exit == clikit.ExitUsage {
		t.Errorf("remote transport should not raise the container USAGE error: %s", ferr.Message)
	}
}
