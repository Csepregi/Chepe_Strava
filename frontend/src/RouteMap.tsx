import { useEffect, useRef, useState } from 'react';
import type { ProfilePoint } from './ElevationProfile';

type RouteMapProps = {
  polyline: string;
  title: string;
  focusPoint?: Coordinate | null;
  streamPoints?: ProfilePoint[];
  onFocusPointChange?: (point: ProfilePoint | null) => void;
};

type Coordinate = {
  lat: number;
  lng: number;
};

const mapboxToken = import.meta.env.VITE_MAPBOX_ACCESS_TOKEN || '';
const routeSourceID = 'strava-route';
const routeLayerID = 'strava-route-line';

function hasCoordinate(point: ProfilePoint): point is ProfilePoint & Required<Pick<ProfilePoint, 'lat' | 'lng'>> {
  return typeof point.lat === 'number' && Number.isFinite(point.lat) && typeof point.lng === 'number' && Number.isFinite(point.lng);
}

function nearestPointForEvent(map: any, points: Array<ProfilePoint & Required<Pick<ProfilePoint, 'lat' | 'lng'>>>, eventPoint: { x: number; y: number }) {
  let nearest: (ProfilePoint & Required<Pick<ProfilePoint, 'lat' | 'lng'>>) | null = null;
  let nearestDistance = Number.POSITIVE_INFINITY;

  for (const point of points) {
    const projected = map.project([point.lng, point.lat]);
    const deltaX = projected.x - eventPoint.x;
    const deltaY = projected.y - eventPoint.y;
    const distance = deltaX * deltaX + deltaY * deltaY;

    if (distance < nearestDistance) {
      nearestDistance = distance;
      nearest = point;
    }
  }

  return nearest;
}

function decodePolyline(encoded: string): Coordinate[] {
  const coordinates: Coordinate[] = [];
  let index = 0;
  let lat = 0;
  let lng = 0;

  while (index < encoded.length) {
    let result = 0;
    let shift = 0;
    let byte = 0;

    do {
      byte = encoded.charCodeAt(index++) - 63;
      result |= (byte & 0x1f) << shift;
      shift += 5;
    } while (byte >= 0x20);

    lat += result & 1 ? ~(result >> 1) : result >> 1;

    result = 0;
    shift = 0;

    do {
      byte = encoded.charCodeAt(index++) - 63;
      result |= (byte & 0x1f) << shift;
      shift += 5;
    } while (byte >= 0x20);

    lng += result & 1 ? ~(result >> 1) : result >> 1;
    coordinates.push({ lat: lat * 1e-5, lng: lng * 1e-5 });
  }

  return coordinates;
}

function routeGeoJSON(coordinates: Coordinate[]): GeoJSON.Feature<GeoJSON.LineString> {
  return {
    type: 'Feature',
    geometry: {
      type: 'LineString',
      coordinates: coordinates.map((coordinate) => [coordinate.lng, coordinate.lat]),
    },
    properties: {},
  };
}

export default function RouteMap({
  polyline,
  title,
  focusPoint = null,
  streamPoints = [],
  onFocusPointChange,
}: RouteMapProps) {
  const mapNodeRef = useRef<HTMLDivElement | null>(null);
  const mapRef = useRef<any>(null);
  const mapboxRef = useRef<any>(null);
  const focusMarkerRef = useRef<any>(null);
  const [mapReady, setMapReady] = useState(false);

  useEffect(() => {
    if (!mapNodeRef.current || !mapboxToken || !polyline) {
      return;
    }

    const coordinates = decodePolyline(polyline);
    if (coordinates.length < 2) {
      return;
    }
    let disposed = false;
    let cleanup: (() => void) | undefined;

    void (async () => {
      const [{ default: mapboxgl }] = await Promise.all([
        import('mapbox-gl'),
        import('mapbox-gl/dist/mapbox-gl.css'),
      ]);

      if (disposed || !mapNodeRef.current) {
        return;
      }

      mapboxgl.accessToken = mapboxToken;
      mapboxRef.current = mapboxgl;

      const map = new mapboxgl.Map({
        container: mapNodeRef.current,
        style: 'mapbox://styles/mapbox/outdoors-v12',
        attributionControl: false,
        cooperativeGestures: true,
      });
      mapRef.current = map;

      map.addControl(new mapboxgl.NavigationControl({ showCompass: false }), 'top-right');

      map.on('load', () => {
        map.addSource(routeSourceID, {
          type: 'geojson',
          data: routeGeoJSON(coordinates),
        });

        map.addLayer({
          id: routeLayerID,
          type: 'line',
          source: routeSourceID,
          layout: {
            'line-cap': 'round',
            'line-join': 'round',
          },
          paint: {
            'line-color': '#fc4c02',
            'line-width': 4,
            'line-opacity': 0.92,
          },
        });

        const bounds = new mapboxgl.LngLatBounds();
        for (const coordinate of coordinates) {
          bounds.extend([coordinate.lng, coordinate.lat]);
        }

        map.fitBounds(bounds, {
          padding: 36,
          duration: 0,
        });
        setMapReady(true);
      });

      cleanup = () => {
        focusMarkerRef.current?.remove();
        focusMarkerRef.current = null;
        mapRef.current = null;
        mapboxRef.current = null;
        setMapReady(false);
        map.remove();
      };
    })();

    return () => {
      disposed = true;
      cleanup?.();
    };
  }, [polyline]);

  useEffect(() => {
    if (!mapReady || !mapRef.current || !mapboxRef.current) {
      return;
    }

    if (!focusPoint) {
      focusMarkerRef.current?.remove();
      focusMarkerRef.current = null;
      return;
    }

    const { lat, lng } = focusPoint;
    if (!Number.isFinite(lat) || !Number.isFinite(lng)) {
      focusMarkerRef.current?.remove();
      focusMarkerRef.current = null;
      return;
    }

    if (!focusMarkerRef.current) {
      focusMarkerRef.current = new mapboxRef.current.Marker({ color: '#1b3044', scale: 0.9 })
        .setLngLat([lng, lat])
        .addTo(mapRef.current);
      return;
    }

    focusMarkerRef.current.setLngLat([lng, lat]);
  }, [focusPoint, mapReady]);

  useEffect(() => {
    if (!mapReady || !mapRef.current || !onFocusPointChange) {
      return;
    }

    const points = streamPoints.filter(hasCoordinate);
    if (points.length === 0) {
      return;
    }

    const map = mapRef.current;

    const handlePointerFocus = (event: any) => {
      const nearest = nearestPointForEvent(map, points, event.point);
      onFocusPointChange(nearest);
    };

    const handlePointerEnter = () => {
      map.getCanvas().style.cursor = 'pointer';
    };

    const handlePointerLeave = () => {
      map.getCanvas().style.cursor = '';
      onFocusPointChange(null);
    };

    map.on('mouseenter', routeLayerID, handlePointerEnter);
    map.on('mousemove', routeLayerID, handlePointerFocus);
    map.on('touchstart', routeLayerID, handlePointerFocus);
    map.on('touchmove', routeLayerID, handlePointerFocus);
    map.on('mouseleave', routeLayerID, handlePointerLeave);

    return () => {
      map.off('mouseenter', routeLayerID, handlePointerEnter);
      map.off('mousemove', routeLayerID, handlePointerFocus);
      map.off('touchstart', routeLayerID, handlePointerFocus);
      map.off('touchmove', routeLayerID, handlePointerFocus);
      map.off('mouseleave', routeLayerID, handlePointerLeave);
      map.getCanvas().style.cursor = '';
    };
  }, [mapReady, onFocusPointChange, streamPoints]);

  if (!mapboxToken) {
    return <div className="route-map-empty">Add `VITE_MAPBOX_ACCESS_TOKEN` to render the interactive route map.</div>;
  }

  if (!polyline) {
    return <div className="route-map-empty">Map preview unavailable for this route.</div>;
  }

  return <div className="route-map route-map-live" ref={mapNodeRef} role="img" aria-label={`${title} route map`} />;
}
