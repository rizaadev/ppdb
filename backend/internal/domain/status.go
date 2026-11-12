package domain

// ApplicationStatus mirrors the PostgreSQL application_status enum and the 17-state admission pipeline.
type ApplicationStatus string

const (
	StatusAccountCreated      ApplicationStatus = "account_created"
	StatusTrackSelected       ApplicationStatus = "track_selected"
	StatusAwaitingPayment     ApplicationStatus = "awaiting_payment"
	StatusPaymentVerified     ApplicationStatus = "payment_verified"
	StatusDraftForm           ApplicationStatus = "draft_form"
	StatusSubmitted           ApplicationStatus = "submitted"
	StatusUnderVerification   ApplicationStatus = "under_verification"
	StatusRevisionRequired    ApplicationStatus = "revision_required"
	StatusVerified            ApplicationStatus = "verified"
	StatusSelectionInProgress ApplicationStatus = "selection_in_progress"
	StatusRanked              ApplicationStatus = "ranked"
	StatusAccepted            ApplicationStatus = "accepted"
	StatusWaitlisted          ApplicationStatus = "waitlisted"
	StatusRejected            ApplicationStatus = "rejected"
	StatusReEnrollment        ApplicationStatus = "re_enrollment"
	StatusEnrolled            ApplicationStatus = "enrolled"
	StatusWithdrawn           ApplicationStatus = "withdrawn"
)

var allowedTransitions = map[ApplicationStatus][]ApplicationStatus{
	StatusAccountCreated:      {StatusTrackSelected, StatusWithdrawn},
	StatusTrackSelected:       {StatusAwaitingPayment, StatusDraftForm, StatusWithdrawn},
	StatusAwaitingPayment:     {StatusPaymentVerified, StatusWithdrawn},
	StatusPaymentVerified:     {StatusDraftForm, StatusWithdrawn},
	StatusDraftForm:           {StatusSubmitted, StatusWithdrawn},
	StatusSubmitted:           {StatusUnderVerification, StatusWithdrawn},
	StatusUnderVerification:   {StatusRevisionRequired, StatusVerified, StatusWithdrawn},
	StatusRevisionRequired:    {StatusSubmitted, StatusWithdrawn},
	StatusVerified:            {StatusSelectionInProgress, StatusRanked, StatusAccepted, StatusWaitlisted, StatusRejected, StatusWithdrawn},
	StatusSelectionInProgress: {StatusRanked, StatusAccepted, StatusWaitlisted, StatusRejected, StatusWithdrawn},
	StatusRanked:              {StatusAccepted, StatusWaitlisted, StatusRejected},
	StatusWaitlisted:          {StatusAccepted, StatusRejected, StatusWithdrawn},
	StatusAccepted:            {StatusReEnrollment, StatusEnrolled, StatusWithdrawn},
	StatusReEnrollment:        {StatusEnrolled, StatusWithdrawn},
}

// CanTransition returns true if moving an application from `from` to `to` is valid in the admission state machine.
func CanTransition(from, to ApplicationStatus) bool {
	targets, ok := allowedTransitions[from]
	if !ok {
		return false
	}
	for _, candidate := range targets {
		if candidate == to {
			return true
		}
	}
	return false
}

// DocumentReviewStatus mirrors the PostgreSQL document_review_status enum.
type DocumentReviewStatus string

const (
	DocStatusPending            DocumentReviewStatus = "pending"
	DocStatusApproved           DocumentReviewStatus = "approved"
	DocStatusRejectedForRevision DocumentReviewStatus = "rejected_for_revision"
)

// IsValidDocumentReviewStatus reports whether raw matches a supported document review decision.
func IsValidDocumentReviewStatus(raw string) bool {
	switch DocumentReviewStatus(raw) {
	case DocStatusPending, DocStatusApproved, DocStatusRejectedForRevision:
		return true
	default:
		return false
	}
}

