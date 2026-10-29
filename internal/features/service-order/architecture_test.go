package serviceorder

import (
	"os/exec"
	"strings"
	"testing"
)

// The dependency rule of specs/service-order-architecture-refactor/
// (requirements.md BR1-BR3, BR5, BR7, §7.5), checked over the transitive
// import graph that `go list -deps` resolves: an indirect path through this
// module's own code (domain -> some internal/shared helper -> net/http) fails
// exactly like a direct import would. Standard-library and third-party
// packages are leaves: what an explicitly allowed library uses internally
// (github.com/google/uuid imports encoding/json for NullUUID) is not this
// module's dependency, and the allowlists below vet which libraries may be
// reached at all. Being a test, it runs with every `go test ./...`, CI's
// build job included.
//
// valueTypePackages are the one exception inside this module: pure value types
// the domain is allowed to use (requirements.md §7.7). They are leaves like a
// third-party library — money.Cents marshals itself to JSON, and that
// encoding/json import is the type's own business — and
// TestValueTypePackagesStayInfrastructureFree keeps them standard-library-only
// so the exception cannot become a path to infrastructure.

const (
	module      = "automotive-workshop-api"
	featureRoot = module + "/internal/features/service-order"
)

// forbiddenForInnerLayers is infrastructure neither domain nor usecase may
// reach. Entries ending in "/" are prefixes.
var forbiddenForInnerLayers = []string{
	"net/http",
	"encoding/json",
	"github.com/jackc/pgx/",
	module + "/internal/shared/apierror",
	module + "/internal/shared/httpx",
	module + "/internal/shared/database",
	module + "/internal/shared/middleware",
	featureRoot + "/adapter/",
}

// valueTypePackages may be imported by domain and usecase and are not traversed
// (see the file comment).
var valueTypePackages = []string{
	module + "/internal/shared/money",
}

func TestDomainDependsOnlyOnStandardLibraryAndUUID(t *testing.T) {
	graph := loadImportGraph(t, "./domain")
	root := featureRoot + "/domain"

	assertNoForbidden(t, graph, root, append(forbiddenForInnerLayers, featureRoot+"/usecase"))
	assertOnlyAllowedNonStandard(t, graph, root, append([]string{
		root,
		"github.com/google/uuid",
	}, valueTypePackages...))
}

func TestUseCaseDependsOnlyOnDomainAndInfrastructureFreeHelpers(t *testing.T) {
	graph := loadImportGraph(t, "./usecase")
	root := featureRoot + "/usecase"

	assertNoForbidden(t, graph, root, forbiddenForInnerLayers)
	assertOnlyAllowedNonStandard(t, graph, root, append([]string{
		root,
		featureRoot + "/domain",
		"github.com/google/uuid",
		module + "/internal/shared/document",
		module + "/internal/shared/trackingtoken",
	}, valueTypePackages...))
}

// TestValueTypePackagesStayInfrastructureFree guards the valueTypePackages
// exception: such a package may import only the standard library, and none of
// its networking, database or process packages. If it ever needs more, it is
// no longer a pure value type and must leave the list.
func TestValueTypePackagesStayInfrastructureFree(t *testing.T) {
	forbiddenStandard := []string{"net/", "database/sql", "os/exec", "syscall"}
	for _, pkg := range valueTypePackages {
		graph := loadImportGraph(t, pkg)
		for _, dependency := range graph.imports[pkg] {
			if !graph.standard[dependency] {
				t.Errorf("value type package %s must import only the standard library, but imports %s", pkg, dependency)
				continue
			}
			for _, rule := range forbiddenStandard {
				if matches(dependency, rule) {
					t.Errorf("value type package %s must stay infrastructure-free, but imports %s", pkg, dependency)
				}
			}
		}
	}
}

// TestAdaptersDoNotDependOnEachOtherOrOnOtherFeatures keeps the adapters
// independent of one another (each points inward only) and the whole slice
// free of other features' packages (CLAUDE.md §9.2).
func TestAdaptersDoNotDependOnEachOtherOrOnOtherFeatures(t *testing.T) {
	adapters := []string{"postgres", "httpapi", "notifier"}
	for _, adapter := range adapters {
		root := featureRoot + "/adapter/" + adapter
		graph := loadImportGraph(t, "./adapter/"+adapter)

		var forbidden []string
		for _, other := range adapters {
			if other != adapter {
				forbidden = append(forbidden, featureRoot+"/adapter/"+other)
			}
		}
		assertNoForbidden(t, graph, root, forbidden)
		assertNoOtherFeature(t, graph, root)
	}
}

// importGraph maps every package in the transitive closure of a root to its
// direct imports, plus whether it belongs to the standard library.
type importGraph struct {
	imports  map[string][]string
	standard map[string]bool
}

func loadImportGraph(t *testing.T, pattern string) importGraph {
	t.Helper()
	output, err := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}|{{.Standard}}|{{join .Imports \" \"}}", pattern).Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			t.Fatalf("go list %s: %v\n%s", pattern, err, exitErr.Stderr)
		}
		t.Fatalf("go list %s: %v", pattern, err)
	}

	graph := importGraph{imports: map[string][]string{}, standard: map[string]bool{}}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "|", 3)
		if len(parts) != 3 {
			t.Fatalf("unexpected go list line %q", line)
		}
		graph.standard[parts[0]] = parts[1] == "true"
		graph.imports[parts[0]] = strings.Fields(parts[2])
	}
	return graph
}

func matches(path, rule string) bool {
	if strings.HasSuffix(rule, "/") {
		return strings.HasPrefix(path, rule) || path == strings.TrimSuffix(rule, "/")
	}
	return path == rule
}

func assertNoForbidden(t *testing.T, graph importGraph, root string, forbidden []string) {
	t.Helper()
	for dependency := range graph.reachable(root) {
		for _, rule := range forbidden {
			if matches(dependency, rule) {
				t.Errorf("%s must not depend on %s; import path: %s", root, dependency, graph.path(root, dependency))
			}
		}
	}
}

func assertOnlyAllowedNonStandard(t *testing.T, graph importGraph, root string, allowed []string) {
	t.Helper()
	for dependency := range graph.reachable(root) {
		if graph.standard[dependency] {
			continue
		}
		permitted := false
		for _, rule := range allowed {
			if matches(dependency, rule) {
				permitted = true
				break
			}
		}
		if !permitted {
			t.Errorf("%s may only depend on the standard library and %v, but reaches %s; import path: %s",
				root, allowed, dependency, graph.path(root, dependency))
		}
	}
}

func assertNoOtherFeature(t *testing.T, graph importGraph, root string) {
	t.Helper()
	for dependency := range graph.reachable(root) {
		if strings.HasPrefix(dependency, module+"/internal/features/") && !matches(dependency, featureRoot+"/") {
			t.Errorf("%s must not depend on another feature's package %s; import path: %s", root, dependency, graph.path(root, dependency))
		}
	}
}

// ownPackage reports whether path is this module's code whose imports the
// checks follow: every package of the module except the valueTypePackages
// leaves.
func ownPackage(path string) bool {
	if !strings.HasPrefix(path, module+"/") {
		return false
	}
	for _, leaf := range valueTypePackages {
		if path == leaf {
			return false
		}
	}
	return true
}

// reachable returns every package reachable from root by following imports
// out of this module's own packages only (see the file comment).
func (graph importGraph) reachable(root string) map[string]bool {
	seen := map[string]bool{root: true}
	queue := []string{root}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if !ownPackage(current) {
			continue
		}
		for _, next := range graph.imports[current] {
			if !seen[next] {
				seen[next] = true
				queue = append(queue, next)
			}
		}
	}
	return seen
}

// path returns one shortest import chain from root to target, so a failure
// names the offending intermediate package instead of only the endpoint.
func (graph importGraph) path(root, target string) string {
	previous := map[string]string{root: ""}
	queue := []string{root}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current == target {
			chain := []string{current}
			for step := previous[current]; step != ""; step = previous[step] {
				chain = append([]string{step}, chain...)
			}
			return strings.Join(chain, " -> ")
		}
		if !ownPackage(current) {
			continue
		}
		for _, next := range graph.imports[current] {
			if _, seen := previous[next]; !seen {
				previous[next] = current
				queue = append(queue, next)
			}
		}
	}
	return root + " -> ... -> " + target
}
