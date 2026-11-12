package domain

import "testing"

func TestCanTransition(t *testing.T) {
	tests := []struct {
		name string
		from ApplicationStatus
		to   ApplicationStatus
		want bool
	}{
		{"free track skips payment", StatusTrackSelected, StatusDraftForm, true},
		{"paid track awaits payment", StatusTrackSelected, StatusAwaitingPayment, true},
		{"draft submits for verification", StatusDraftForm, StatusSubmitted, true},
		{"verifier requests revision", StatusUnderVerification, StatusRevisionRequired, true},
		{"applicant resubmits revision", StatusRevisionRequired, StatusSubmitted, true},
		{"waitlist promoted to accepted", StatusWaitlisted, StatusAccepted, true},
		{"accepted enrolls directly when free", StatusAccepted, StatusEnrolled, true},
		{"cannot jump from draft to accepted", StatusDraftForm, StatusAccepted, false},
		{"withdrawn is terminal", StatusWithdrawn, StatusDraftForm, false},
		{"enrolled is terminal", StatusEnrolled, StatusAccepted, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := CanTransition(tc.from, tc.to); got != tc.want {
				t.Fatalf("CanTransition(%q, %q) = %v, want %v", tc.from, tc.to, got, tc.want)
			}
		})
	}
}

