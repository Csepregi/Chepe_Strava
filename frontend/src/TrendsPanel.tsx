import { useMemo, useState } from 'react';

export type TrendBucket = {
  label: string;
  start: string;
  end: string;
  activity_count: number;
  distance_meters: number;
  moving_time_seconds: number;
  elevation_gain: number;
  average_pace_seconds_per_km?: number | null;
};

export type TrendRecords = {
  best_pace_seconds_per_km?: number | null;
  most_elevation_gain: number;
  highest_max_speed: number;
  longest_duration_seconds: number;
};

export type TrendsResponse = {
  weekly: TrendBucket[];
  monthly: TrendBucket[];
  records: TrendRecords;
};

type TrendsPanelProps = {
  data: TrendsResponse | null;
  loading?: boolean;
  error?: string;
};

type TrendMetric = 'pace' | 'distance' | 'elevation';
type TrendInterval = 'weekly' | 'monthly';

type ChartPoint = {
  label: string;
  value: number;
  formattedValue: string;
  activityCount: number;
};

type ChartScale = {
  ticks: number[];
  heightFor: (value: number) => number;
};

function formatPace(secondsPerKm: number | null | undefined): string {
  if (!secondsPerKm || !Number.isFinite(secondsPerKm) || secondsPerKm <= 0) {
    return '-';
  }

  const totalSeconds = Math.max(0, Math.round(secondsPerKm));
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = (totalSeconds % 60).toString().padStart(2, '0');
  return `${minutes}:${seconds} /km`;
}

function formatDistance(meters: number): string {
  return `${(meters / 1000).toFixed(1)} km`;
}

function formatElevation(meters: number): string {
  return `${Math.round(meters)} m`;
}

function formatSpeed(metersPerSecond: number): string {
  return `${(metersPerSecond * 3.6).toFixed(1)} km/h`;
}

function formatDuration(seconds: number): string {
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const remainingSeconds = seconds % 60;
  return `${hours}:${minutes.toString().padStart(2, '0')}:${remainingSeconds.toString().padStart(2, '0')} hrs`;
}

function metricValue(bucket: TrendBucket, metric: TrendMetric): number {
  if (metric === 'pace') {
    return bucket.average_pace_seconds_per_km ?? 0;
  }
  if (metric === 'distance') {
    return bucket.distance_meters / 1000;
  }
  return bucket.elevation_gain;
}

function metricLabel(bucket: TrendBucket, metric: TrendMetric): string {
  if (metric === 'pace') {
    return formatPace(bucket.average_pace_seconds_per_km);
  }
  if (metric === 'distance') {
    return formatDistance(bucket.distance_meters);
  }
  return formatElevation(bucket.elevation_gain);
}

function metricTick(value: number, metric: TrendMetric): string {
  if (metric === 'pace') {
    return formatPace(value);
  }
  if (metric === 'distance') {
    return `${value.toFixed(1)} km`;
  }
  return `${Math.round(value)} m`;
}

export default function TrendsPanel({ data, loading = false, error = '' }: TrendsPanelProps) {
  const [interval, setInterval] = useState<TrendInterval>('weekly');
  const [metric, setMetric] = useState<TrendMetric>('distance');

  const series = interval === 'weekly' ? data?.weekly ?? [] : data?.monthly ?? [];

  const chartPoints = useMemo<ChartPoint[]>(
    () =>
      series.map((bucket) => ({
        label: bucket.label,
        value: metricValue(bucket, metric),
        formattedValue: metricLabel(bucket, metric),
        activityCount: bucket.activity_count,
      })),
    [metric, series],
  );

  const chartScale = useMemo<ChartScale>(() => {
    const values = chartPoints.map((point) => point.value).filter((value) => Number.isFinite(value) && value > 0);
    if (metric === 'pace') {
      const observedMin = values.length > 0 ? Math.min(...values) : 300;
      const observedMax = values.length > 0 ? Math.max(...values) : observedMin;
      const range = Math.max(observedMax - observedMin, 1);
      const lowerPadding = Math.max(range * 0.08, 4);
      const upperPadding = Math.max(range * 0.24, 12);
      const domainMin = Math.max(observedMin - lowerPadding, 0);
      const domainMax = observedMax + upperPadding;
      const span = Math.max(domainMax - domainMin, 1);

      return {
        ticks: Array.from({ length: 5 }, (_, index) => domainMin + (span / 4) * index),
        heightFor: (value: number) => {
          if (!Number.isFinite(value) || value <= 0) {
            return 0;
          }
          return ((domainMax - value) / span) * 100;
        },
      };
    }

    const observedMax = values.length > 0 ? Math.max(...values) : 1;
    const domainMax = Math.max(observedMax * 1.08, 1);

    return {
      ticks: Array.from({ length: 5 }, (_, index) => domainMax - (domainMax / 4) * index),
      heightFor: (value: number) => {
        if (!Number.isFinite(value) || value <= 0) {
          return 0;
        }
        return (value / domainMax) * 100;
      },
    };
  }, [chartPoints, metric]);

  const recordItems = [
    {
      label: 'Best pace',
      value: formatPace(data?.records.best_pace_seconds_per_km),
    },
    {
      label: 'Most elevation',
      value: formatElevation(data?.records.most_elevation_gain ?? 0),
    },
    {
      label: 'Highest max speed',
      value: formatSpeed(data?.records.highest_max_speed ?? 0),
    },
    {
      label: 'Longest duration',
      value: formatDuration(data?.records.longest_duration_seconds ?? 0),
    },
  ];

  return (
    <section className="panel trends-panel">
      <div className="trends-layout">
        <aside className="records-panel">
          <p className="kicker">Effort</p>
          <h2>Records</h2>
          <div className="records-list">
            {recordItems.map((item) => (
              <div key={item.label} className="record-item">
                <p>{item.label}</p>
                <strong>{item.value}</strong>
              </div>
            ))}
          </div>
        </aside>

        <div className="trend-chart-card">
          <div className="trend-card-head">
            <div>
              <p className="kicker">Diligence</p>
              <h2>Adventure trends</h2>
            </div>
            <div className="trend-toggle-groups">
              <div className="toggle-group">
                <button className={`toggle-pill ${interval === 'weekly' ? 'active' : ''}`} type="button" onClick={() => setInterval('weekly')}>
                  Weekly
                </button>
                <button className={`toggle-pill ${interval === 'monthly' ? 'active' : ''}`} type="button" onClick={() => setInterval('monthly')}>
                  Monthly
                </button>
              </div>

              <div className="toggle-group">
                <button className={`toggle-pill ${metric === 'pace' ? 'active' : ''}`} type="button" onClick={() => setMetric('pace')}>
                  Pace
                </button>
                <button className={`toggle-pill ${metric === 'distance' ? 'active' : ''}`} type="button" onClick={() => setMetric('distance')}>
                  Distance
                </button>
                <button className={`toggle-pill ${metric === 'elevation' ? 'active' : ''}`} type="button" onClick={() => setMetric('elevation')}>
                  Elevation
                </button>
              </div>
            </div>
          </div>

          {error && <p className="message error">{error}</p>}
          {loading ? <p className="subtle">Loading trend data...</p> : null}

          {!loading && chartPoints.length === 0 ? (
            <div className="trend-empty">No cached activity data is available for trends yet.</div>
          ) : null}

          {!loading && chartPoints.length > 0 ? (
            <div className="trend-chart-shell">
              <div className="trend-y-axis">
                {chartScale.ticks.map((tick) => (
                  <span key={tick}>{metricTick(tick, metric)}</span>
                ))}
              </div>

              <div className="trend-plot-shell">
                <div className="trend-grid-lines">
                  {chartScale.ticks.map((tick) => (
                    <span key={tick} />
                  ))}
                </div>

                <div className="trend-bars">
                  {chartPoints.map((point) => {
                    const height = chartScale.heightFor(point.value);
                    return (
                      <div key={`${interval}-${metric}-${point.label}`} className="trend-bar-column">
                        <button
                          className="trend-bar-hitbox"
                          type="button"
                          aria-label={`${point.label}: ${point.formattedValue}, ${point.activityCount} activities`}
                        >
                          <span className="trend-tooltip">
                            <strong>{point.label}</strong>
                            <span>{point.formattedValue}</span>
                            <small>
                              {point.activityCount} activit{point.activityCount === 1 ? 'y' : 'ies'}
                            </small>
                          </span>
                          <span className={`trend-bar-fill trend-bar-fill-${metric}`} style={{ height: `${Math.max(height, point.value > 0 ? 6 : 0)}%` }} />
                        </button>
                        <span className="trend-x-label">{point.label}</span>
                      </div>
                    );
                  })}
                </div>
              </div>
            </div>
          ) : null}
        </div>
      </div>
    </section>
  );
}
