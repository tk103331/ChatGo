package acpregistry

import (
	"strings"
	"testing"
)

// registryFixture is a representative slice of the published index: an npx
// entry, a uvx entry, a binary-only entry, and one with no usable runtime.
const registryFixture = `{
  "version": "1.0.0",
  "agents": [
    {
      "id": "binary-only",
      "name": "Binary Only",
      "version": "1.0.0",
      "distribution": {
        "binary": {
          "darwin-aarch64": {
            "archive": "https://example.com/b.tar.gz",
            "sha256": "cafe",
            "cmd": "./b"
          }
        }
      }
    },
    {
      "id": "codex-acp",
      "name": "Codex",
      "version": "2.6.0",
      "description": "ACP adapter for OpenAI's coding assistant",
      "license": "Apache-2.0",
      "distribution": {
        "npx": {
          "package": "@agentclientprotocol/codex-acp@2.6.0",
          "args": ["--acp"]
        }
      }
    },
    {
      "id": "fast-agent",
      "name": "fast-agent",
      "version": "0.10.1",
      "distribution": {
        "uvx": {
          "package": "fast-agent-acp==0.10.1",
          "args": ["-x"],
          "env": {"FAST_AGENT_MODEL": "codexplan"}
        }
      }
    },
    {"id": "no-name", "version": "1.0.0", "distribution": {"npx": {"package": "x@1.0.0"}}},
    {"id": "empty", "name": "Empty", "version": "1.0.0"}
  ]
}`

func parseFixture(t *testing.T) Registry {
	t.Helper()
	reg, err := Parse(strings.NewReader(registryFixture))
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

func findAgent(reg Registry, id string) Agent {
	for _, a := range reg.Agents {
		if a.ID == id {
			return a
		}
	}
	return Agent{}
}

// Parse keeps only entries with a name and an id, sorted by name.
func TestParseFiltersAndSorts(t *testing.T) {
	reg := parseFixture(t)
	if len(reg.Agents) != 3 {
		t.Fatalf("the unusable entries should be dropped, got %+v", reg.Agents)
	}
	for i := 1; i < len(reg.Agents); i++ {
		if reg.Agents[i-1].Name > reg.Agents[i].Name {
			t.Fatalf("the agents should be sorted by name: %v", reg.Agents)
		}
	}
}

// An npx entry maps to npx -y <pinned package> with the entry's own args and
// env; a uvx entry maps the same way. A binary-only entry maps to nothing.
func TestCommandMapsTheDistributions(t *testing.T) {
	reg := parseFixture(t)

	codex := findAgent(reg, "codex-acp")
	command, args, env, ok := codex.Command()
	if !ok || command != "npx" {
		t.Fatalf("the npx entry should run through npx, got %q %v", command, args)
	}
	want := []string{"-y", "@agentclientprotocol/codex-acp@2.6.0", "--acp"}
	if strings.Join(args, " ") != strings.Join(want, " ") {
		t.Fatalf("args = %v, want %v", args, want)
	}
	if len(env) != 0 {
		t.Fatalf("the entry carries no env, got %v", env)
	}

	fast := findAgent(reg, "fast-agent")
	command, args, env, ok = fast.Command()
	if !ok || command != "uvx" {
		t.Fatalf("the uvx entry should run through uvx, got %q %v", command, args)
	}
	want = []string{"fast-agent-acp==0.10.1", "-x"}
	if strings.Join(args, " ") != strings.Join(want, " ") {
		t.Fatalf("args = %v, want %v", args, want)
	}
	if env["FAST_AGENT_MODEL"] != "codexplan" {
		t.Fatalf("the entry's env should carry over, got %v", env)
	}

	binary := findAgent(reg, "binary-only")
	if _, _, _, ok := binary.Command(); ok {
		t.Fatal("a binary-only entry should not map to a command")
	}
	if !binary.BinaryOnly() {
		t.Fatal("the entry without a runtime should read as binary-only")
	}
	if findAgent(reg, "codex-acp").BinaryOnly() {
		t.Fatal("an npx entry is not binary-only")
	}
}

// PkgName strips the pinned version of both runtimes' specs, so a
// connection added from an older pin still matches the registry entry.
func TestPkgNameStripsThePin(t *testing.T) {
	cases := map[string]string{
		"@agentclientprotocol/codex-acp@2.6.0": "@agentclientprotocol/codex-acp",
		"@agentclientprotocol/codex-acp":       "@agentclientprotocol/codex-acp",
		"fast-agent-acp==0.10.1":               "fast-agent-acp",
		"cline@3.0.74":                         "cline",
		"cline":                                "cline",
	}
	for in, want := range cases {
		if got := PkgName(in); got != want {
			t.Fatalf("PkgName(%q) = %q, want %q", in, got, want)
		}
	}
}

// PlatformKey maps GOOS/GOARCH onto the index's binary keys, including the
// Apple Silicon key the published index spells with its literal marker.
func TestPlatformKey(t *testing.T) {
	cases := map[[2]string]string{
		{"darwin", "arm64"}:  darwinArm64Key,
		{"darwin", "amd64"}:  "darwin-x86_64",
		{"linux", "amd64"}:   "linux-x86_64",
		{"linux", "arm64"}:   "linux-aarch64",
		{"windows", "amd64"}: "windows-x86_64",
		{"windows", "arm64"}: "windows-aarch64",
	}
	for in, want := range cases {
		if got := PlatformKey(in[0], in[1]); got != want {
			t.Fatalf("PlatformKey(%v) = %q, want %q", in, got, want)
		}
	}
}

// BinaryPlatforms names the archive targets of an entry in a stable order,
// and PlatformLabel turns the index's keys — the Apple Silicon one with its
// literal marker included — into readable names.
func TestPlatformLabels(t *testing.T) {
	a := Agent{Distribution: Distribution{Binary: map[string]BinaryDist{
		"linux-x86_64": {}, darwinArm64Key: {}, "made-up": {},
	}}}
	got := strings.Join(a.BinaryPlatforms(), " | ")
	want := "macOS · Apple Silicon | Linux · x86_64 | made-up"
	if got != want {
		t.Fatalf("BinaryPlatforms = %q, want %q", got, want)
	}
	if PlatformLabel("darwin-x86_64") != "macOS · Intel" ||
		PlatformLabel("linux-aarch64-musl") != "Linux · ARM64 (musl)" ||
		PlatformLabel("windows-aarch64") != "Windows · ARM64" {
		t.Fatalf("a platform label reads wrong: %q %q %q",
			PlatformLabel("darwin-x86_64"), PlatformLabel("linux-aarch64-musl"),
			PlatformLabel("windows-aarch64"))
	}
}
