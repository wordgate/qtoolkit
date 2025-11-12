package qtoolkit

import "testing"

func TestNumEncoderCaseInsensitive(t *testing.T) {
	// 测试不区分大小写的编码器（之前会导致数组越界）
	encoder, err := NewNumEncoder(6, 819238123123123, false)
	if err != nil {
		t.Fatalf("Failed to create encoder: %v", err)
	}

	// 测试编码一系列 ID（包括可能导致越界的值）
	testIDs := []uint64{1, 10, 50, 67, 68, 100, 1000}
	
	for _, id := range testIDs {
		code, err := encoder.Encode(id)
		if err != nil {
			t.Errorf("Failed to encode ID %d: %v", id, err)
			continue
		}
		
		// 验证编码后的长度
		if len(code) != 6 {
			t.Errorf("Expected code length 6, got %d for ID %d", len(code), id)
		}
		
		// 验证解码后能还原原始 ID
		decoded := encoder.Decode(code)
		if decoded != id {
			t.Errorf("Decode mismatch: encoded %d -> %s -> decoded %d", id, code, decoded)
		}
		
		t.Logf("ID %d -> Code %s -> Decoded %d ✓", id, code, decoded)
	}
}

func TestNumEncoderCaseSensitive(t *testing.T) {
	// 测试区分大小写的编码器（应该继续正常工作）
	encoder, err := NewNumEncoder(6, 819238123123123, true)
	if err != nil {
		t.Fatalf("Failed to create encoder: %v", err)
	}

	testIDs := []uint64{1, 10, 50, 67, 68, 100, 1000}
	
	for _, id := range testIDs {
		code, err := encoder.Encode(id)
		if err != nil {
			t.Errorf("Failed to encode ID %d: %v", id, err)
			continue
		}
		
		decoded := encoder.Decode(code)
		if decoded != id {
			t.Errorf("Decode mismatch: encoded %d -> %s -> decoded %d", id, code, decoded)
		}
		
		t.Logf("ID %d -> Code %s -> Decoded %d ✓", id, code, decoded)
	}
}
