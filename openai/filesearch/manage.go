package filesearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
)

// CreateStore creates a new OpenAI vector store.
// Returns the store ID.
func CreateStore(ctx context.Context, name string) (string, error) {
	cfg := getConfig()
	if cfg.APIKey == "" {
		return "", fmt.Errorf("filesearch: api_key is required")
	}

	reqBody, _ := json.Marshal(map[string]string{"name": name})

	url := baseURL + "/v1/vector_stores"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("filesearch: request error: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("filesearch: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("filesearch: create store failed: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("filesearch: unmarshal error: %w", err)
	}

	return result.ID, nil
}

// UploadFile uploads a file to an OpenAI vector store.
// The file is first uploaded via the Files API, then attached to the vector store.
func UploadFile(ctx context.Context, storeID string, filename string, reader io.Reader) error {
	cfg := getConfig()
	if cfg.APIKey == "" {
		return fmt.Errorf("filesearch: api_key is required")
	}

	// Step 1: Upload file via Files API
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	writer.WriteField("purpose", "assistants")
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return fmt.Errorf("filesearch: create form file error: %w", err)
	}
	if _, err := io.Copy(part, reader); err != nil {
		return fmt.Errorf("filesearch: copy file error: %w", err)
	}
	writer.Close()

	url := baseURL + "/v1/files"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return fmt.Errorf("filesearch: request error: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("filesearch: upload failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("filesearch: upload failed: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	var fileResult struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &fileResult); err != nil {
		return fmt.Errorf("filesearch: unmarshal error: %w", err)
	}

	// Step 2: Attach file to vector store
	attachBody, _ := json.Marshal(map[string]string{"file_id": fileResult.ID})
	url = fmt.Sprintf("%s/v1/vector_stores/%s/files", baseURL, storeID)
	req, err = http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(attachBody))
	if err != nil {
		return fmt.Errorf("filesearch: request error: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp2, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("filesearch: attach file failed: %w", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode >= 300 {
		respBody, _ = io.ReadAll(resp2.Body)
		return fmt.Errorf("filesearch: attach file failed: status %d, body: %s", resp2.StatusCode, string(respBody))
	}

	return nil
}

// Search performs a direct vector search without LLM generation.
// Useful for debugging and testing retrieval quality.
func Search(ctx context.Context, storeName string, query string) ([]Citation, error) {
	cfg := getConfig()
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("filesearch: api_key is required")
	}

	vectorStoreID, _, maxResults, err := resolveStore(cfg, storeName)
	if err != nil {
		return nil, err
	}

	reqBody, _ := json.Marshal(map[string]any{
		"query":       query,
		"max_results": maxResults,
	})

	url := fmt.Sprintf("%s/v1/vector_stores/%s/search", baseURL, vectorStoreID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("filesearch: request error: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("filesearch: search failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("filesearch: search failed: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	var searchResp struct {
		Data []struct {
			FileName string  `json:"filename"`
			Score    float64 `json:"score"`
			Content  []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &searchResp); err != nil {
		return nil, fmt.Errorf("filesearch: unmarshal error: %w", err)
	}

	var citations []Citation
	for _, d := range searchResp.Data {
		text := ""
		if len(d.Content) > 0 {
			text = d.Content[0].Text
		}
		citations = append(citations, Citation{
			FileName: d.FileName,
			Score:    d.Score,
			Text:     text,
		})
	}

	return citations, nil
}
