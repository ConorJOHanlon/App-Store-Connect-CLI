package asc

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestRedactedReviewDetailHidesPasswordUnderAnyKeyCase(t *testing.T) {
	const sentinel = "asc-red-sentinel-folded-key-pw-71c2e0"
	var resp AppStoreReviewDetailResponse
	body := `{"data":{"type":"appStoreReviewDetails","id":"detail-1","attributes":{"demoAccountName":null,"DemoAccountPassword":"` + sentinel + `"}}}`
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	output, err := json.Marshal(RedactAppStoreReviewDetailResponse(&resp))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(output), sentinel) {
		t.Fatalf("redacted output leaked the password: %s", output)
	}
	if !strings.Contains(string(output), `"demoAccountPassword":"`+RedactedValuePlaceholder+`"`) ||
		!strings.Contains(string(output), `"demoAccountName":null`) {
		t.Fatalf("expected redacted password and Apple's null name, got %s", output)
	}
}

func TestWriteRequestsDoNotEchoAppleResponseAttributes(t *testing.T) {
	const apple = `{"locale":"en-US","whatsNew":"Fixes","marketingUrl":null,"privacyChoicesUrl":null,"kidsAgeBand":null,"unmodeled":true}`
	decode := func(t *testing.T, target any) {
		t.Helper()
		if err := json.Unmarshal([]byte(apple), target); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
	}
	tests := []struct {
		name string
		call func(*testing.T, *Client) error
	}{
		{"create version localization", func(t *testing.T, c *Client) error {
			var attrs AppStoreVersionLocalizationAttributes
			decode(t, &attrs)
			_, err := c.CreateAppStoreVersionLocalization(context.Background(), "version-1", attrs)
			return err
		}},
		{"update version localization", func(t *testing.T, c *Client) error {
			var attrs AppStoreVersionLocalizationAttributes
			decode(t, &attrs)
			_, err := c.UpdateAppStoreVersionLocalization(context.Background(), "loc-1", attrs)
			return err
		}},
		{"create test notes", func(t *testing.T, c *Client) error {
			var attrs BetaBuildLocalizationAttributes
			decode(t, &attrs)
			_, err := c.CreateBetaBuildLocalization(context.Background(), "build-1", attrs)
			return err
		}},
		{"update test notes", func(t *testing.T, c *Client) error {
			var attrs BetaBuildLocalizationAttributes
			decode(t, &attrs)
			_, err := c.UpdateBetaBuildLocalization(context.Background(), "loc-1", attrs)
			return err
		}},
		{"create app info localization", func(t *testing.T, c *Client) error {
			var attrs AppInfoLocalizationAttributes
			decode(t, &attrs)
			_, err := c.CreateAppInfoLocalization(context.Background(), "info-1", attrs)
			return err
		}},
		{"update app info localization", func(t *testing.T, c *Client) error {
			var attrs AppInfoLocalizationAttributes
			decode(t, &attrs)
			_, err := c.UpdateAppInfoLocalization(context.Background(), "loc-1", attrs)
			return err
		}},
		{"update age rating", func(t *testing.T, c *Client) error {
			var attrs AgeRatingDeclarationAttributes
			decode(t, &attrs)
			_, err := c.UpdateAgeRatingDeclaration(context.Background(), "age-1", attrs)
			return err
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := newTestClient(t, func(req *http.Request) {
				body, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatalf("read body: %v", err)
				}
				if strings.Contains(string(body), "null") || strings.Contains(string(body), "unmodeled") {
					t.Fatalf("request echoed Apple's response attributes: %s", body)
				}
			}, jsonResponse(http.StatusOK, `{"data":{"type":"x","id":"1","attributes":{}}}`))
			if err := test.call(t, client); err != nil {
				t.Fatalf("request error: %v", err)
			}
		})
	}
}
