package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

const enmaasModelsURL = "https://api.enmaas.devshift.net/v1/models"

// EnMaaSMoels returns model IDs advertised by the EnMaaS endpoint. It is a
// best-effort convenience for form suggestions; missing credentials simply
// produce an empty list rather than an error.
func (a *App) EnMaaSMoels(ctx context.Context) ([]string, error) {
	key := os.Getenv("ENMAAS_KEY")
	if key == "" {
		return nil, nil
	}
	client := &http.Client{Timeout: 5 * time.Second}
	return fetchModelIDs(ctx, client, enmaasModelsURL, key)
}

func fetchModelIDs(ctx context.Context, client *http.Client, url, key string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("model discovery failed: HTTP %d", resp.StatusCode)
	}
	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	var ids []string
	for _, model := range body.Data {
		id := strings.TrimSpace(model.ID)
		if id != "" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids, nil
}
