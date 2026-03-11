import { useEffect, useRef } from 'react';

type RouteMapProps = {
  polyline: string;
  title: string;
};

type Coordinate = {
  lat: number;
  lng: number;
};

const mapboxToken = import.meta.env.VITE_MAPBOX_ACCESS_TOKEN || '';
const routeSourceID = 'strava-route';
const routeLayerID = 'strava-route-line';

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

export default function RouteMap({ polyline, title }: RouteMapProps) {
  const mapNodeRef = useRef<HTMLDivElement | null>(null);

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

      const map = new mapboxgl.Map({
        container: mapNodeRef.current,
        style: 'mapbox://styles/mapbox/outdoors-v12',
        attributionControl: false,
        cooperativeGestures: true,
      });

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
      });

      cleanup = () => {
        map.remove();
      };
    })();

    return () => {
      disposed = true;
      cleanup?.();
    };
  }, [polyline]);

  if (!mapboxToken) {
    return <div className="route-map-empty">Add `VITE_MAPBOX_ACCESS_TOKEN` to render the interactive route map.</div>;
  }

  if (!polyline) {
    return <div className="route-map-empty">Map preview unavailable for this route.</div>;
  }

  return <div className="route-map route-map-live" ref={mapNodeRef} role="img" aria-label={`${title} route map`} />;
}
