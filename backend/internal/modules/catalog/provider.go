package catalog

import "github.com/google/uuid"

// Provider supplies tenant/unit/period/track lookups (either static seed or mutable runtime store).
type Provider interface {
	FindTrack(tenantID, unitID, periodID, trackID uuid.UUID) (*TrackLookup, bool)
	FindTrackByID(tenantID, trackID uuid.UUID) (*TrackLookup, bool)
	FindPeriodTracks(tenantID, periodID uuid.UUID) ([]TrackLookup, bool)
	UnitBelongsToTenant(tenantID, unitID uuid.UUID) bool
}

type staticProvider struct{}

// DefaultProvider returns a read-only provider backed by compile-time demo seed helpers.
func DefaultProvider() Provider { return staticProvider{} }

func (staticProvider) FindTrack(tenantID, unitID, periodID, trackID uuid.UUID) (*TrackLookup, bool) {
	return FindTrack(tenantID, unitID, periodID, trackID)
}

func (staticProvider) FindTrackByID(tenantID, trackID uuid.UUID) (*TrackLookup, bool) {
	return FindTrackByID(tenantID, trackID)
}

func (staticProvider) FindPeriodTracks(tenantID, periodID uuid.UUID) ([]TrackLookup, bool) {
	return FindPeriodTracks(tenantID, periodID)
}

func (staticProvider) UnitBelongsToTenant(tenantID, unitID uuid.UUID) bool {
	return UnitBelongsToTenant(tenantID, unitID)
}

