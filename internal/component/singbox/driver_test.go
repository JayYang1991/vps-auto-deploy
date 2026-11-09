package singbox

import (
	"encoding/base64"
	"testing"
)

func TestDeriveRealityPublicKey(t *testing.T) {
	// 1. 生成一组真实密钥对并验证推导一致性
	priv, expectedPub, _, err := GenerateRealityKeypair()
	if err != nil {
		t.Fatalf("GenerateRealityKeypair failed: %v", err)
	}

	derivedPub := DeriveRealityPublicKey(priv)
	if derivedPub != expectedPub {
		t.Fatalf("derived public key mismatch: got %s, want %s", derivedPub, expectedPub)
	}

	// 2. 测试带标准 Base64 (含填充) 的私钥
	rawPriv, err := base64.RawURLEncoding.DecodeString(priv)
	if err != nil {
		t.Fatalf("decode raw priv failed: %v", err)
	}
	stdPrivWithPadding := base64.StdEncoding.EncodeToString(rawPriv)
	derivedFromStd := DeriveRealityPublicKey(stdPrivWithPadding)
	if derivedFromStd != expectedPub {
		t.Fatalf("derived public key from std base64 mismatch: got %s, want %s", derivedFromStd, expectedPub)
	}

	// 3. 空白与非法输入测试
	if got := DeriveRealityPublicKey(""); got != "" {
		t.Errorf("expected empty string for empty input, got %q", got)
	}
	if got := DeriveRealityPublicKey("   "); got != "" {
		t.Errorf("expected empty string for whitespace input, got %q", got)
	}
	if got := DeriveRealityPublicKey("invalid-base64!!!"); got != "" {
		t.Errorf("expected empty string for invalid base64, got %q", got)
	}
	// 非 32 字节
	tooShort := base64.RawURLEncoding.EncodeToString([]byte("short"))
	if got := DeriveRealityPublicKey(tooShort); got != "" {
		t.Errorf("expected empty string for non-32-byte key, got %q", got)
	}

	// 4. 首尾空白字符容错
	derivedWithSpaces := DeriveRealityPublicKey("  " + priv + "\n")
	if derivedWithSpaces != expectedPub {
		t.Fatalf("derived public key with spaces mismatch: got %s, want %s", derivedWithSpaces, expectedPub)
	}
}
