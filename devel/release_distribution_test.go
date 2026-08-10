package devel_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNativePackageReleaseMetadata(t *testing.T) {
	t.Parallel()

	configuration := readRepositoryFile(t, "../.goreleaser.yml")
	for _, required := range []string{
		"nfpms:",
		"package_name: jobman-diagnose",
		"artifacts: package",
		"- apk",
		"- deb",
		"- rpm",
		"/usr/share/licenses/jobman-diagnose/LICENSE",
		"/usr/share/doc/jobman-diagnose/CONFIGURATION.md",
	} {
		if !strings.Contains(configuration, required) {
			t.Errorf("GoReleaser configuration is missing %q", required)
		}
	}
}

func TestCloudsmithWorkflowUsesProtectedAPIKey(t *testing.T) {
	t.Parallel()

	workflow := readRepositoryFile(t, "../.github/workflows/publish-cloudsmith-packages.yml")
	for _, required := range []string{
		"types:\n      - published",
		"workflow_dispatch:",
		"name: main",
		"cloudsmith-io/cloudsmith-cli-action@db783de9f6e7a445e5e31d94f4210303b48a10a3",
		"api-key: ${{ secrets.CLOUDSMITH_API_KEY }}",
		"cli-version: 1.21.0",
		`verify-auth: "true"`,
		"publish-cloudsmith-packages.sh",
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("Cloudsmith workflow is missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"id-token: write",
		"oidc-namespace:",
		"oidc-service-slug:",
	} {
		if strings.Contains(workflow, forbidden) {
			t.Errorf("Cloudsmith workflow contains unsupported OIDC configuration %q", forbidden)
		}
	}
}

func TestCloudsmithPublicationVerifiesReleaseAndIsIdempotent(t *testing.T) {
	t.Parallel()

	helper := readRepositoryFile(t, "publish-cloudsmith-packages.sh")
	for _, required := range []string{
		"CLOUDSMITH_API_KEY is required",
		"gh release view",
		"git merge-base --is-ancestor",
		"cosign verify-blob",
		"sha256sum --check",
		"gh attestation verify",
		"jobman/stable",
		"any-distro/any-version",
		"alpine/any-version",
		"source-sha256-",
		"Cloudsmith already contains verified",
	} {
		if !strings.Contains(helper, required) {
			t.Errorf("Cloudsmith helper is missing %q", required)
		}
	}
	assertExecutable(t, "publish-cloudsmith-packages.sh")
}

func TestCloudsmithPublicationRejectsMissingCredentials(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		environment []string
		want        string
	}{
		{name: "GitHub token", want: "GH_TOKEN is required"},
		{
			name:        "Cloudsmith key",
			environment: []string{"GH_TOKEN=test-token"},
			want:        "CLOUDSMITH_API_KEY is required",
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			command := exec.CommandContext( // #nosec G204 -- The executable and arguments are repository-controlled.
				t.Context(),
				"bash",
				"./publish-cloudsmith-packages.sh",
				"v1.2.3",
			)
			command.Dir = "."
			command.Env = append([]string{"PATH=" + os.Getenv("PATH")}, testCase.environment...)
			output, err := command.CombinedOutput()
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != 2 {
				t.Fatalf("exit error = %v, want status 2\n%s", err, output)
			}
			if !strings.Contains(string(output), testCase.want) {
				t.Fatalf("output = %q, want %q", output, testCase.want)
			}
		})
	}
}

func TestHomebrewWorkflowPublishesVerifiedFormula(t *testing.T) {
	t.Parallel()

	workflow := readRepositoryFile(t, "../.github/workflows/publish-homebrew-formula.yml")
	for _, required := range []string{
		"types:\n      - published",
		"workflow_dispatch:",
		"name: main",
		"secrets.HOMEBREW_TAP_TOKEN",
		"gh api repos/ryancswallace/homebrew-tap",
		"repository: ryancswallace/homebrew-tap",
		"cosign verify-blob",
		"gh attestation verify",
		"go run ./devel/homebrewformula",
		"Formula/jobman-diagnose.rb",
		"gh pr create",
		"gh pr merge \"${pr_url}\" --auto --squash --delete-branch",
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("Homebrew workflow is missing %q", required)
		}
	}
	if strings.Contains(workflow, "git push origin HEAD:main") {
		t.Error("Homebrew workflow bypasses tap pull-request validation")
	}
}

func TestReleaseWorkflowChecksCompleteArtifactSet(t *testing.T) {
	t.Parallel()

	workflow := readRepositoryFile(t, "../.github/workflows/release.yml")
	if !strings.Contains(workflow, "run: ./devel/check-release.sh dist signed") {
		t.Error("release workflow does not validate the complete artifact set")
	}
	if !strings.Contains(workflow, "run: ./devel/package-smoke.sh dist") {
		t.Error("release workflow does not install native packages in target distributions")
	}
	assertExecutable(t, "check-release.sh")
	assertExecutable(t, "package-smoke.sh")
}

func TestStagedReleaseWorkflowVerifiesBeforePublishingByID(t *testing.T) {
	t.Parallel()

	workflow := readRepositoryFile(t, "../.github/workflows/publish-staged-release.yml")
	for _, required := range []string{
		"actions: write",
		"./devel/check-release.sh dist signed",
		"cosign verify-blob",
		"gh attestation verify",
		"source-ref \"refs/tags/${RELEASE_TAG}\"",
		`"${target_commit}" != "main"`,
		"verify-bin/jobman-diagnose --version",
		`"repos/${GITHUB_REPOSITORY}/releases/${RELEASE_ID}"`,
		"-F draft=false",
		"gh workflow run publish-homebrew-formula.yml",
		"gh workflow run publish-cloudsmith-packages.yml",
		`-f "tag=${RELEASE_TAG}"`,
	} {
		if !strings.Contains(workflow, required) {
			t.Errorf("staged release workflow is missing %q", required)
		}
	}
}

func TestProtectedEnvironmentAllowsMainAndReleaseTags(t *testing.T) {
	t.Parallel()

	settings := readRepositoryFile(t, "../.github/settings.yml")
	for _, required := range []string{
		"- name: main\n          type: branch",
		`- name: "v*.*.*"` + "\n          type: tag",
	} {
		if !strings.Contains(settings, required) {
			t.Errorf("repository settings are missing deployment policy %q", required)
		}
	}
}

func readRepositoryFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path) // #nosec G304 -- Tests pass repository-controlled paths.
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}

func assertExecutable(t *testing.T, name string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(name)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("%s is not executable", filepath.Base(name))
	}
}
