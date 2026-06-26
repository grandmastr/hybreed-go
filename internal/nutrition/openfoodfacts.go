package nutrition

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Open Food Facts is a free, open product database with broad barcode coverage.
// We use it as a fallback when a scanned barcode isn't already in our foods
// table, then persist the hit locally so the next scan is a DB lookup.
//
// Docs: https://openfoodfacts.github.io/openfoodfacts-server/api/
const offProductURL = "https://world.openfoodfacts.org/api/v2/product/"

var offClient = &http.Client{Timeout: 6 * time.Second}

// offFood is a normalised Open Food Facts product (macros per 100 g).
type offFood struct {
	Name     string
	Kcal     int32
	ProteinG float64
	CarbsG   float64
	FatG     float64
}

type offResponse struct {
	Status  int `json:"status"` // 1 = found, 0 = not found
	Product struct {
		ProductName string `json:"product_name"`
		Brands      string `json:"brands"`
		Nutriments  struct {
			EnergyKcal100g float64 `json:"energy-kcal_100g"`
			Proteins100g   float64 `json:"proteins_100g"`
			Carbs100g      float64 `json:"carbohydrates_100g"`
			Fat100g        float64 `json:"fat_100g"`
		} `json:"nutriments"`
	} `json:"product"`
}

// lookupOpenFoodFacts fetches a product by barcode. Returns (nil, nil) when Open
// Food Facts has no usable product for the code (not an error).
func lookupOpenFoodFacts(ctx context.Context, code string) (*offFood, error) {
	url := offProductURL + code + ".json?fields=product_name,brands,nutriments"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	// OFF asks API clients to identify themselves.
	req.Header.Set("User-Agent", "Hybreed/1.0 (+https://hybreed.app)")

	resp, err := offClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("open food facts: status %d", resp.StatusCode)
	}

	var r offResponse
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(r.Product.ProductName)
	if r.Status != 1 || name == "" {
		return nil, nil
	}
	if brand := strings.TrimSpace(r.Product.Brands); brand != "" {
		name = fmt.Sprintf("%s (%s)", name, strings.TrimSpace(strings.SplitN(brand, ",", 2)[0]))
	}
	n := r.Product.Nutriments
	return &offFood{
		Name:     name,
		Kcal:     int32(n.EnergyKcal100g + 0.5),
		ProteinG: n.Proteins100g,
		CarbsG:   n.Carbs100g,
		FatG:     n.Fat100g,
	}, nil
}
