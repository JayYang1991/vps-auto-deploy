package upgrade

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestParseELFArch(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skip("unable to get current test executable")
	}

	arch, err := ParseELFArch(exe)
	if err != nil {
		t.Fatalf("ParseELFArch failed on current executable: %v", err)
	}

	if arch != runtime.GOARCH {
		t.Errorf("expected arch %s, got %s", runtime.GOARCH, arch)
	}
}

func TestComputeFileSHA256(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test-sha-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	_, _ = tmpFile.WriteString("hello vps-panel upgrade")
	_ = tmpFile.Close()

	hash, err := ComputeFileSHA256(tmpFile.Name())
	if err != nil {
		t.Fatalf("ComputeFileSHA256 failed: %v", err)
	}

	if len(hash) != 64 {
		t.Errorf("expected 64-char hex string, got %d (%s)", len(hash), hash)
	}
}

func TestInspectAndSavePackageWithTarGz(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skip("unable to get current test executable")
	}

	tempDir, err := os.MkdirTemp("", "test-pkg-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// 打包当前可执行文件为 tar.gz
	tarGzPath := filepath.Join(tempDir, "vps-panel-linux-amd64.tar.gz")
	tarFile, err := os.Create(tarGzPath)
	if err != nil {
		t.Fatal(err)
	}

	gw := gzip.NewWriter(tarFile)
	tw := tar.NewWriter(gw)

	exeData, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}

	hdr := &tar.Header{
		Name: "vps-panel",
		Mode: 0755,
		Size: int64(len(exeData)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(exeData); err != nil {
		t.Fatal(err)
	}
	_ = tw.Close()
	_ = gw.Close()
	_ = tarFile.Close()

	info, err := InspectAndSavePackage(tarGzPath, "vps-panel-linux-amd64.tar.gz")
	if err != nil {
		t.Fatalf("InspectAndSavePackage failed: %v", err)
	}

	if info.Arch != runtime.GOARCH {
		t.Errorf("expected arch %s, got %s", runtime.GOARCH, info.Arch)
	}

	if len(info.SHA256) != 64 {
		t.Errorf("expected 64-char sha256, got %s", info.SHA256)
	}

	// 检查 ListPackages
	list, err := ListPackages()
	if err != nil {
		t.Fatalf("ListPackages failed: %v", err)
	}
	if len(list) == 0 {
		t.Errorf("expected at least 1 package in list")
	}

	// 测试寻找最佳包
	binPath, sha, err := FindBestPackageForArch(runtime.GOARCH)
	if err != nil {
		t.Fatalf("FindBestPackageForArch failed: %v", err)
	}
	if binPath == "" || len(sha) != 64 {
		t.Errorf("invalid best package: binPath=%s, sha=%s", binPath, sha)
	}
}

