package domain

import "math"

const earthRadiusMeters = 6371000.0

// HaversineMeters calculates the great-circle distance in meters between two WGS84 coordinate pairs.
func HaversineMeters(lat1, lon1, lat2, lon2 float64) int {
	phi1 := lat1 * math.Pi / 180.0
	phi2 := lat2 * math.Pi / 180.0
	deltaPhi := (lat2 - lat1) * math.Pi / 180.0
	deltaLambda := (lon2 - lon1) * math.Pi / 180.0

	sinHalfPhi := math.Sin(deltaPhi / 2.0)
	sinHalfLambda := math.Sin(deltaLambda / 2.0)

	a := sinHalfPhi*sinHalfPhi + math.Cos(phi1)*math.Cos(phi2)*sinHalfLambda*sinHalfLambda
	c := 2.0 * math.Atan2(math.Sqrt(a), math.Sqrt(1.0-a))
	return int(math.Round(earthRadiusMeters * c))
}

