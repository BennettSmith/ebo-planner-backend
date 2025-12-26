package itest

import (
	"net/http"
	"testing"
)

func TestMembers_ITest(t *testing.T) {
	for _, b := range backendsFromEnv(t) {
		t.Run(string(b), func(t *testing.T) {
			srv := newTestServer(t, b)

			// Missing auth header => 401
			{
				status, body, hdr := srv.doJSON(t, http.MethodGet, "/members/me", "", nil)
				_ = hdr
				requireErrorCode(t, status, body, http.StatusUnauthorized, "UNAUTHORIZED")
			}

			subject := "itest|alice"

			// Many endpoints require a provisioned member; before provisioning, directory access is blocked.
			{
				status, body, _ := srv.doJSON(t, http.MethodGet, "/members", subject, nil)
				requireErrorCode(t, status, body, http.StatusUnauthorized, "MEMBER_NOT_PROVISIONED")
			}

			// Provision the member for this subject.
			var created struct {
				Member struct {
					MemberId    string `json:"memberId"`
					DisplayName string `json:"displayName"`
					Email       string `json:"email"`
				} `json:"member"`
			}
			{
				status, body, _ := srv.doJSON(t, http.MethodPost, "/members", subject, map[string]any{
					"displayName": "Alice",
					"email":       "alice@example.com",
				})
				if status != http.StatusCreated {
					t.Fatalf("status=%d want=%d body=%s", status, http.StatusCreated, string(body))
				}
				created = mustUnmarshal[struct {
					Member struct {
						MemberId    string `json:"memberId"`
						DisplayName string `json:"displayName"`
						Email       string `json:"email"`
					} `json:"member"`
				}](t, body)
				if created.Member.MemberId == "" {
					t.Fatalf("expected memberId to be set; body=%s", string(body))
				}
			}

			// Get my profile now succeeds.
			{
				status, body, _ := srv.doJSON(t, http.MethodGet, "/members/me", subject, nil)
				if status != http.StatusOK {
					t.Fatalf("status=%d want=%d body=%s", status, http.StatusOK, string(body))
				}
				var got struct {
					Member struct {
						MemberId string `json:"memberId"`
					} `json:"member"`
				}
				got = mustUnmarshal[struct {
					Member struct {
						MemberId string `json:"memberId"`
					} `json:"member"`
				}](t, body)
				if got.Member.MemberId != created.Member.MemberId {
					t.Fatalf("memberId=%q want=%q body=%s", got.Member.MemberId, created.Member.MemberId, string(body))
				}
			}

			// Directory access now succeeds.
			{
				status, body, _ := srv.doJSON(t, http.MethodGet, "/members", subject, nil)
				if status != http.StatusOK {
					t.Fatalf("status=%d want=%d body=%s", status, http.StatusOK, string(body))
				}
				var list struct {
					Members []struct {
						MemberId    string `json:"memberId"`
						DisplayName string `json:"displayName"`
					} `json:"members"`
				}
				list = mustUnmarshal[struct {
					Members []struct {
						MemberId    string `json:"memberId"`
						DisplayName string `json:"displayName"`
					} `json:"members"`
				}](t, body)

				found := false
				for _, m := range list.Members {
					if m.MemberId == created.Member.MemberId {
						found = true
						if m.DisplayName != "Alice" {
							t.Fatalf("displayName=%q want=%q", m.DisplayName, "Alice")
						}
					}
				}
				if !found {
					t.Fatalf("expected created member in list; body=%s", string(body))
				}
			}
		})
	}
}
