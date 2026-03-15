import { useId, useMemo, useState } from 'react';

export type ProfilePoint = {
  distance_meters: number;
  altitude_meters: number;
  lat?: number | null;
  lng?: number | null;
};

export type ProfileSummaryCard = {
  label: string;
  value: string;
  detail?: string;
};

type ElevationProfileProps = {
  points: ProfilePoint[];
  title: string;
  focusedPoint?: ProfilePoint | null;
  onFocusPointChange?: (point: ProfilePoint | null) => void;
  summaryCards?: ProfileSummaryCard[];
};

type ProfileAnnotation = {
  id: string;
  label: string;
  distance_meters: number;
  altitude_meters: number;
};

const viewWidth = 920;
const viewHeight = 320;
const padding = { top: 88, right: 22, bottom: 54, left: 52 };
const footerBandHeight = 28;

function clamp(value: number, min: number, max: number): number {
  return Math.min(Math.max(value, min), max);
}

function formatDistance(distanceMeters: number): string {
  return `${(distanceMeters / 1000).toFixed(distanceMeters >= 100000 ? 0 : 1)} km`;
}

function formatAltitude(altitudeMeters: number): string {
  return `${Math.round(altitudeMeters)} m`;
}

function buildAltitudeTicks(minAltitude: number, maxAltitude: number): number[] {
  const tickCount = 5;
  const rawStep = Math.max((maxAltitude - minAltitude) / tickCount, 1);
  const roundedStep = Math.max(Math.round(rawStep / 50) * 50, 50);
  const start = Math.floor(minAltitude / roundedStep) * roundedStep;
  const end = Math.ceil(maxAltitude / roundedStep) * roundedStep;
  const ticks: number[] = [];

  for (let tick = start; tick <= end; tick += roundedStep) {
    ticks.push(tick);
  }

  return ticks;
}

function detectAnnotations(points: ProfilePoint[]): ProfileAnnotation[] {
  if (points.length < 8) {
    return [];
  }

  const maxDistance = points[points.length - 1]?.distance_meters ?? 0;
  const altitudes = points.map((point) => point.altitude_meters);
  const minAltitude = Math.min(...altitudes);
  const maxAltitude = Math.max(...altitudes);
  const altitudeSpan = Math.max(maxAltitude - minAltitude, 1);
  const windowSize = Math.max(8, Math.floor(points.length / 18));
  const prominenceThreshold = Math.max(45, altitudeSpan * 0.12);
  const minDistanceBetween = Math.max(3500, maxDistance * 0.12);

  const candidates: Array<ProfileAnnotation & { prominence: number }> = [];

  for (let index = 1; index < points.length - 1; index += 1) {
    const previousAltitude = points[index - 1].altitude_meters;
    const currentAltitude = points[index].altitude_meters;
    const nextAltitude = points[index + 1].altitude_meters;
    const isPeak = (currentAltitude >= previousAltitude && currentAltitude > nextAltitude) || (currentAltitude > previousAltitude && currentAltitude >= nextAltitude);
    if (!isPeak) {
      continue;
    }

    const leftStart = Math.max(0, index - windowSize);
    const rightEnd = Math.min(points.length - 1, index + windowSize);
    let leftLowIndex = leftStart;
    let rightLowIndex = index;

    for (let cursor = leftStart; cursor <= index; cursor += 1) {
      if (points[cursor].altitude_meters <= points[leftLowIndex].altitude_meters) {
        leftLowIndex = cursor;
      }
    }

    for (let cursor = index; cursor <= rightEnd; cursor += 1) {
      if (points[cursor].altitude_meters <= points[rightLowIndex].altitude_meters) {
        rightLowIndex = cursor;
      }
    }

    const prominence = currentAltitude - Math.max(points[leftLowIndex].altitude_meters, points[rightLowIndex].altitude_meters);
    if (prominence < prominenceThreshold) {
      continue;
    }

    candidates.push({
      id: `peak-${index}`,
      label: 'Climb',
      distance_meters: points[index].distance_meters,
      altitude_meters: currentAltitude,
      prominence,
    });
  }

  const highestPointIndex = points.reduce(
    (bestIndex, point, index) => (point.altitude_meters > points[bestIndex].altitude_meters ? index : bestIndex),
    0,
  );
  const highestPoint = points[highestPointIndex];
  const highestCandidate = {
    id: `peak-highest-${highestPointIndex}`,
    label: 'High point',
    distance_meters: highestPoint.distance_meters,
    altitude_meters: highestPoint.altitude_meters,
    prominence: highestPoint.altitude_meters - minAltitude,
  };
  candidates.push(highestCandidate);

  candidates.sort((left, right) => right.prominence - left.prominence || right.altitude_meters - left.altitude_meters);

  const selected: Array<ProfileAnnotation & { prominence: number }> = [highestCandidate];
  for (const candidate of candidates) {
    if (candidate.id === highestCandidate.id) {
      continue;
    }
    if (selected.some((existing) => Math.abs(existing.distance_meters - candidate.distance_meters) < minDistanceBetween)) {
      continue;
    }
    selected.push(candidate);
    if (selected.length === 3) {
      break;
    }
  }

  selected.sort((left, right) => left.distance_meters - right.distance_meters);

  return selected.map((annotation, index) => ({
    ...annotation,
    label: annotation.id.startsWith('peak-highest') ? 'High point' : `Climb ${index + 1}`,
  }));
}

export default function ElevationProfile({
  points,
  title,
  focusedPoint = null,
  onFocusPointChange,
  summaryCards,
}: ElevationProfileProps) {
  const [activeIndex, setActiveIndex] = useState<number | null>(null);
  const gradientId = useId().replace(/:/g, '');
  const patternId = `${gradientId}-contours`;

  const metrics = useMemo(() => {
    if (points.length < 2) {
      return null;
    }

    const distances = points.map((point) => point.distance_meters);
    const altitudes = points.map((point) => point.altitude_meters);
    const maxDistance = Math.max(...distances);
    const minAltitude = Math.min(...altitudes);
    const maxAltitude = Math.max(...altitudes);
    const altitudeSpan = Math.max(maxAltitude - minAltitude, 1);
    const innerWidth = viewWidth - padding.left - padding.right;
    const plotBottom = viewHeight - padding.bottom;
    const innerHeight = plotBottom - padding.top;

    const xForDistance = (distanceMeters: number) => padding.left + (distanceMeters / Math.max(maxDistance, 1)) * innerWidth;
    const yForAltitude = (altitudeMeters: number) =>
      padding.top + innerHeight - ((altitudeMeters - minAltitude) / altitudeSpan) * innerHeight;

    const linePath = points
      .map((point, index) => `${index === 0 ? 'M' : 'L'} ${xForDistance(point.distance_meters)} ${yForAltitude(point.altitude_meters)}`)
      .join(' ');

    const areaPath = `${linePath} L ${xForDistance(points[points.length - 1].distance_meters)} ${plotBottom} L ${xForDistance(
      points[0].distance_meters,
    )} ${plotBottom} Z`;

    const highestPoint = points.reduce(
      (bestPoint, point) => (point.altitude_meters > bestPoint.altitude_meters ? point : bestPoint),
      points[0],
    );

    return {
      maxDistance,
      minAltitude,
      maxAltitude,
      plotBottom,
      xForDistance,
      yForAltitude,
      linePath,
      areaPath,
      altitudeTicks: buildAltitudeTicks(minAltitude, maxAltitude),
      annotations: detectAnnotations(points),
      highestPoint,
    };
  }, [points]);

  if (!metrics) {
    return <div className="profile-empty">Elevation profile unavailable for this selection.</div>;
  }

  const activePoint = focusedPoint ?? (activeIndex === null ? null : points[activeIndex] ?? null);
  const activeX = activePoint ? metrics.xForDistance(activePoint.distance_meters) : null;
  const activeY = activePoint ? metrics.yForAltitude(activePoint.altitude_meters) : null;

  const defaultSummaryCards: ProfileSummaryCard[] = [
    {
      label: 'Start',
      value: formatAltitude(points[0].altitude_meters),
      detail: '0.0 km',
    },
    {
      label: 'High point',
      value: formatAltitude(metrics.highestPoint.altitude_meters),
      detail: formatDistance(metrics.highestPoint.distance_meters),
    },
    {
      label: 'Finish',
      value: formatAltitude(points[points.length - 1].altitude_meters),
      detail: formatDistance(metrics.maxDistance),
    },
    {
      label: 'Distance',
      value: formatDistance(metrics.maxDistance),
      detail: `${formatAltitude(metrics.minAltitude)} to ${formatAltitude(metrics.maxAltitude)}`,
    },
  ];

  const updateActiveIndex = (clientX: number, currentTarget: HTMLDivElement) => {
    const bounds = currentTarget.getBoundingClientRect();
    const ratio = clamp((clientX - bounds.left) / Math.max(bounds.width, 1), 0, 1);
    const index = Math.round(ratio * (points.length - 1));
    setActiveIndex(index);
    onFocusPointChange?.(points[index] ?? null);
  };

  return (
    <div
      className="profile-shell profile-shell-rich"
      onPointerLeave={() => {
        setActiveIndex(null);
        onFocusPointChange?.(null);
      }}
      onPointerMove={(event) => updateActiveIndex(event.clientX, event.currentTarget)}
      onPointerDown={(event) => updateActiveIndex(event.clientX, event.currentTarget)}
    >
      <div className="profile-head">
        <div>
          <p className="route-visual-label">Elevation Profile</p>
          <strong>{title}</strong>
        </div>
        <div className="profile-head-values">
          {activePoint ? (
            <>
              <span>{formatDistance(activePoint.distance_meters)}</span>
              <span>{formatAltitude(activePoint.altitude_meters)}</span>
            </>
          ) : (
            <>
              <span>{formatDistance(metrics.maxDistance)}</span>
              <span>{formatAltitude(metrics.highestPoint.altitude_meters)} high point</span>
            </>
          )}
        </div>
      </div>

      <svg className="profile-chart profile-chart-rich" viewBox={`0 0 ${viewWidth} ${viewHeight}`} role="img" aria-label={`${title} elevation profile`}>
        <defs>
          <linearGradient id={gradientId} x1="0%" x2="0%" y1="0%" y2="100%">
            <stop offset="0%" stopColor="#8ea48f" stopOpacity="0.78" />
            <stop offset="100%" stopColor="#dfe7da" stopOpacity="0.16" />
          </linearGradient>
          <pattern id={patternId} width="14" height="12" patternUnits="userSpaceOnUse">
            <path d="M 0 11 H 14" stroke="rgba(48, 78, 67, 0.14)" strokeWidth="1" />
          </pattern>
        </defs>

        {metrics.altitudeTicks.map((tick) => {
          const y = metrics.yForAltitude(tick);
          return (
            <g key={tick}>
              <line x1={padding.left} y1={y} x2={viewWidth - padding.right} y2={y} className="profile-grid" />
              <text x={padding.left - 12} y={y + 4} textAnchor="end" className="profile-axis-label">
                {Math.round(tick)} m
              </text>
            </g>
          );
        })}

        <path d={metrics.areaPath} fill={`url(#${gradientId})`} className="profile-area" />
        <path d={metrics.areaPath} fill={`url(#${patternId})`} className="profile-area-contours" />
        <path d={metrics.linePath} className="profile-line" />

        {metrics.annotations.map((annotation) => {
          const annotationX = metrics.xForDistance(annotation.distance_meters);
          const annotationY = metrics.yForAltitude(annotation.altitude_meters);
          return (
            <g key={annotation.id}>
              <line x1={annotationX} y1={padding.top - 6} x2={annotationX} y2={metrics.plotBottom} className="profile-annotation-line" />
              <circle cx={annotationX} cy={annotationY} r="4.5" className="profile-annotation-dot" />
              <text transform={`translate(${annotationX + 8} ${padding.top - 10}) rotate(-74)`} className="profile-annotation-text">
                {annotation.label} · {formatAltitude(annotation.altitude_meters)}
              </text>
            </g>
          );
        })}

        {activePoint && activeX !== null && activeY !== null && (
          <>
            <line x1={activeX} x2={activeX} y1={padding.top - 8} y2={metrics.plotBottom} className="profile-guide" />
            <circle cx={activeX} cy={activeY} r="5.5" className="profile-marker" />
          </>
        )}

        <rect
          x={padding.left}
          y={viewHeight - footerBandHeight}
          width={viewWidth - padding.left - padding.right}
          height={footerBandHeight}
          rx="12"
          className="profile-footer-band"
        />
        <text x={padding.left + 12} y={viewHeight - 10} className="profile-footer-text">
          Start · 0 km
        </text>
        <text x={viewWidth - padding.right - 12} y={viewHeight - 10} textAnchor="end" className="profile-footer-text">
          Finish · {formatDistance(metrics.maxDistance)}
        </text>
      </svg>

      <div className="profile-summary">
        {(summaryCards ?? defaultSummaryCards).map((card) => (
          <div key={card.label} className="profile-summary-card">
            <p className="profile-summary-label">{card.label}</p>
            <strong>{card.value}</strong>
            {card.detail ? <span className="profile-summary-detail">{card.detail}</span> : null}
          </div>
        ))}
      </div>
    </div>
  );
}
