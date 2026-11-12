import { useEffect, useRef, useState } from 'react';
import L from 'leaflet';
import 'leaflet/dist/leaflet.css';
import { LocateFixed, MapPin } from 'lucide-react';
import { Button } from '@/components/ui/Button';
import { calculateHaversineMeters, formatDistanceLabel } from '@/lib/geo';
import styles from './LocationPickerMap.module.css';

export interface Coordinates {
  latitude: number;
  longitude: number;
}

export function LocationPickerMap({
  value,
  schoolCoords,
  schoolName,
  onChange,
}: {
  value: Coordinates;
  schoolCoords?: Coordinates;
  schoolName?: string;
  onChange: (next: Coordinates) => void;
}) {
  const mapElRef = useRef<HTMLDivElement | null>(null);
  const mapRef = useRef<L.Map | null>(null);
  const homeMarkerRef = useRef<L.Marker | null>(null);
  const lineRef = useRef<L.Polyline | null>(null);
  const [locating, setLocating] = useState(false);
  const [geoError, setGeoError] = useState<string | null>(null);

  useEffect(() => {
    if (!mapElRef.current || mapRef.current) return;

    const map = L.map(mapElRef.current).setView([value.latitude, value.longitude], 15);
    L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
      attribution: '&copy; OpenStreetMap contributors',
      maxZoom: 19,
    }).addTo(map);

    const homeIcon = L.divIcon({
      className: 'pbdb-map-pin',
      iconSize: [22, 22],
      iconAnchor: [11, 22],
    });

    const marker = L.marker([value.latitude, value.longitude], {
      draggable: true,
      icon: homeIcon,
    }).addTo(map);

    marker.bindTooltip('Lokasi Rumah (geser pin jika kurang tepat)');
    marker.on('dragend', () => {
      const pos = marker.getLatLng();
      onChange({
        latitude: Number(pos.lat.toFixed(6)),
        longitude: Number(pos.lng.toFixed(6)),
      });
    });

    map.on('click', (event: L.LeafletMouseEvent) => {
      onChange({
        latitude: Number(event.latlng.lat.toFixed(6)),
        longitude: Number(event.latlng.lng.toFixed(6)),
      });
    });

    if (schoolCoords) {
      const schoolIcon = L.divIcon({
        className: 'pbdb-school-pin',
        iconSize: [18, 18],
        iconAnchor: [9, 9],
      });
      L.marker([schoolCoords.latitude, schoolCoords.longitude], { icon: schoolIcon })
        .addTo(map)
        .bindTooltip(schoolName ?? 'Titik Sekolah');
    }

    mapRef.current = map;
    homeMarkerRef.current = marker;

    return () => {
      map.remove();
      mapRef.current = null;
      homeMarkerRef.current = null;
      lineRef.current = null;
    };
    // Initialize map once on mount.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    const map = mapRef.current;
    const marker = homeMarkerRef.current;
    if (!map || !marker) return;

    marker.setLatLng([value.latitude, value.longitude]);

    if (lineRef.current) {
      lineRef.current.remove();
      lineRef.current = null;
    }
    if (schoolCoords) {
      lineRef.current = L.polyline(
        [
          [value.latitude, value.longitude],
          [schoolCoords.latitude, schoolCoords.longitude],
        ],
        { color: '#1e3a5f', weight: 2, dashArray: '6 6' },
      ).addTo(map);
    }
  }, [value.latitude, value.longitude, schoolCoords]);

  function handleDetectGPS() {
    if (!('geolocation' in navigator)) {
      setGeoError('Perangkat/browser tidak mendukung GPS otomatis. Silakan ketuk atau geser pin di peta.');
      return;
    }
    setLocating(true);
    setGeoError(null);
    navigator.geolocation.getCurrentPosition(
      (pos) => {
        const next = {
          latitude: Number(pos.coords.latitude.toFixed(6)),
          longitude: Number(pos.coords.longitude.toFixed(6)),
        };
        onChange(next);
        mapRef.current?.setView([next.latitude, next.longitude], 16);
        setLocating(false);
      },
      () => {
        setGeoError('Izin lokasi ditolak atau sinyal lemah. Geser pin secara manual pada peta di bawah.');
        setLocating(false);
      },
      { enableHighAccuracy: true, timeout: 8000 },
    );
  }

  const distanceMeters = schoolCoords
    ? calculateHaversineMeters(
        value.latitude,
        value.longitude,
        schoolCoords.latitude,
        schoolCoords.longitude,
      )
    : null;

  return (
    <div className={styles.container}>
      <div className={styles.toolbar}>
        <Button variant="secondary" size="sm" onClick={handleDetectGPS} disabled={locating}>
          <LocateFixed size={16} />
          {locating ? 'Mencari koordinat...' : 'Gunakan Lokasi Saat Ini (GPS)'}
        </Button>

        {distanceMeters !== null && (
          <div className={`${styles.distancePill} tabular-nums`}>
            <MapPin size={14} />
            Estimasi Jarak: {formatDistanceLabel(distanceMeters)}
          </div>
        )}
      </div>

      {geoError && (
        <div className={styles.errorBox} role="alert">
          {geoError}
        </div>
      )}

      <div ref={mapElRef} className={styles.mapCanvas} aria-label="Peta titik koordinat domisili" />

      <div className={styles.coordsFooter}>
        <span className="mono-code">
          Lat: {value.latitude.toFixed(6)}, Lon: {value.longitude.toFixed(6)}
        </span>
        <span>Perkiraan jarak indikatif; jarak resmi dihitung oleh server.</span>
      </div>
    </div>
  );
}

