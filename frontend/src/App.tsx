import { useEffect, useMemo, useState } from 'react';
import ElevationProfile, { type ProfilePoint } from './ElevationProfile';
import RouteMap from './RouteMap';

type Athlete = {
  id: number;
  firstname: string;
  lastname: string;
  username: string;
  city: string;
  state: string;
  country: string;
  profile_medium: string;
};

type MeResponse = {
  athlete: Athlete;
  token_expires_at: string;
};

type Activity = {
  id: number;
  athlete_id: number;
  name: string;
  type: string;
  distance_meters: number;
  moving_time_seconds: number;
  elapsed_time_seconds: number;
  total_elevation_gain: number;
  average_speed: number;
  max_speed: number;
  start_date: string;
  timezone: string;
  map_polyline: string;
  summary_polyline: string;
};

type SyncResponse = {
  fetched_activities: number;
  pages_fetched: number;
  after_days: number;
};

type RouteSyncResponse = {
  synced_routes: number;
  skipped_routes: number;
};

type LatestActivityResponse = {
  activity: Activity | null;
};

type ActivityVisualResponse = {
  description: string;
  photo_count: number;
  primary_photo_url: string;
  primary_photo_thumbnail_url: string;
  stream_points: ProfilePoint[];
};

type WeekOption = {
  key: string;
  start: Date;
  label: string;
};

type WeeklySummary = {
  totalActivities: number;
  totalDistanceKm: number;
  totalMovingHours: number;
  totalElevationGain: number;
  activeDays: number;
};

type ActivityTone = {
  label: string;
  short: string;
  tone: 'run' | 'ride' | 'walk' | 'hike' | 'swim' | 'workout' | 'default';
};

const activityFetchLimit = 200;
const rawBase = import.meta.env.VITE_API_BASE_URL || '';
const apiBase = rawBase.endsWith('/') ? rawBase.slice(0, -1) : rawBase;
const apiUrl = (path: string) => `${apiBase}${path}`;

async function apiRequest<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(apiUrl(path), {
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      ...(init?.headers || {}),
    },
    ...init,
  });

  if (response.status === 401) {
    throw new Error('unauthorized');
  }

  if (!response.ok) {
    const payload = await response.json().catch(() => ({ error: 'Unknown error' }));
    throw new Error(payload.error || 'Request failed');
  }

  return response.json() as Promise<T>;
}

function formatDuration(seconds: number): string {
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  return `${hours}h ${minutes}m`;
}

function formatDistance(meters: number): string {
  return `${(meters / 1000).toFixed(1)} km`;
}

function formatPace(distanceMeters: number, movingSeconds: number): string {
  if (distanceMeters <= 0 || movingSeconds <= 0) {
    return '-';
  }
  const secondsPerKm = movingSeconds / (distanceMeters / 1000);
  const paceMin = Math.floor(secondsPerKm / 60);
  const paceSec = Math.round(secondsPerKm % 60)
    .toString()
    .padStart(2, '0');
  return `${paceMin}:${paceSec} /km`;
}

function formatSpeed(metersPerSecond: number): string {
  if (metersPerSecond <= 0) {
    return '-';
  }
  return `${(metersPerSecond * 3.6).toFixed(1)} km/h`;
}

function activityResultKey(activity: Activity): string {
  return `activity:${activity.id}`;
}

function localDateKey(date: Date): string {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, '0');
  const day = String(date.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

function parseLocalDateKey(value: string): Date {
  const [year, month, day] = value.split('-').map(Number);
  return new Date(year, month - 1, day);
}

function startOfWeek(date: Date): Date {
  const next = new Date(date);
  next.setHours(0, 0, 0, 0);
  const day = next.getDay();
  const delta = day === 0 ? -6 : 1 - day;
  next.setDate(next.getDate() + delta);
  return next;
}

function startOfWeekKey(date: Date): string {
  return localDateKey(startOfWeek(date));
}

function addDays(date: Date, days: number): Date {
  const next = new Date(date);
  next.setDate(next.getDate() + days);
  next.setHours(0, 0, 0, 0);
  return next;
}

function formatWeekLabel(start: Date): string {
  const end = addDays(start, 6);
  const sameYear = start.getFullYear() === end.getFullYear();
  const sameMonth = sameYear && start.getMonth() === end.getMonth();

  if (sameMonth) {
    return `${start.toLocaleDateString(undefined, { month: 'long', day: 'numeric' })} - ${end.getDate()}, ${end.getFullYear()}`;
  }

  if (sameYear) {
    return `${start.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })} - ${end.toLocaleDateString(undefined, {
      month: 'short',
      day: 'numeric',
      year: 'numeric',
    })}`;
  }

  return `${start.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' })} - ${end.toLocaleDateString(
    undefined,
    { month: 'short', day: 'numeric', year: 'numeric' },
  )}`;
}

function formatActivityDateTime(value: string): string {
  return new Date(value).toLocaleString(undefined, {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  });
}

function formatActivityTime(value: string): string {
  return new Date(value).toLocaleTimeString(undefined, {
    hour: '2-digit',
    minute: '2-digit',
  });
}

function buildWeekOptions(activities: Activity[], extraWeekKeys: string[]): WeekOption[] {
  const weeks = new Map<string, Date>();

  for (const key of extraWeekKeys) {
    if (!weeks.has(key)) {
      weeks.set(key, parseLocalDateKey(key));
    }
  }

  for (const activity of activities) {
    const weekKey = startOfWeekKey(new Date(activity.start_date));
    if (!weeks.has(weekKey)) {
      weeks.set(weekKey, parseLocalDateKey(weekKey));
    }
  }

  return [...weeks.entries()]
    .sort((left, right) => right[1].getTime() - left[1].getTime())
    .map(([key, start]) => ({
      key,
      start,
      label: formatWeekLabel(start),
    }));
}

function summarizeWeek(activities: Activity[]): WeeklySummary {
  return {
    totalActivities: activities.length,
    totalDistanceKm: activities.reduce((sum, activity) => sum + activity.distance_meters, 0) / 1000,
    totalMovingHours: activities.reduce((sum, activity) => sum + activity.moving_time_seconds, 0) / 3600,
    totalElevationGain: activities.reduce((sum, activity) => sum + activity.total_elevation_gain, 0),
    activeDays: new Set(activities.map((activity) => localDateKey(new Date(activity.start_date)))).size,
  };
}

function activityTypeMeta(type: string): ActivityTone {
  const normalized = type.toLowerCase();

  if (normalized.includes('ride') || normalized.includes('cycle')) {
    return { label: 'Ride', short: 'C', tone: 'ride' };
  }
  if (normalized.includes('run')) {
    return { label: 'Run', short: 'R', tone: 'run' };
  }
  if (normalized.includes('walk')) {
    return { label: 'Walk', short: 'W', tone: 'walk' };
  }
  if (normalized.includes('hike')) {
    return { label: 'Hike', short: 'H', tone: 'hike' };
  }
  if (normalized.includes('swim')) {
    return { label: 'Swim', short: 'S', tone: 'swim' };
  }
  if (normalized.includes('workout') || normalized.includes('weight') || normalized.includes('training')) {
    return { label: 'Workout', short: 'G', tone: 'workout' };
  }

  return {
    label: type || 'Activity',
    short: (type || 'A').slice(0, 1).toUpperCase(),
    tone: 'default',
  };
}

export default function App() {
  const [me, setMe] = useState<MeResponse | null>(null);
  const [activities, setActivities] = useState<Activity[]>([]);
  const [latestActivity, setLatestActivity] = useState<Activity | null>(null);
  const [selectedWeekKey, setSelectedWeekKey] = useState<string>(startOfWeekKey(new Date()));
  const [selectedActivityId, setSelectedActivityId] = useState<number | null>(null);
  const [loading, setLoading] = useState<boolean>(true);
  const [syncing, setSyncing] = useState<boolean>(false);
  const [syncingRoutes, setSyncingRoutes] = useState<boolean>(false);
  const [statusMessage, setStatusMessage] = useState<string>('');
  const [errorMessage, setErrorMessage] = useState<string>('');
  const [activityVisuals, setActivityVisuals] = useState<Record<string, ActivityVisualResponse>>({});
  const [visualLoadingKeys, setVisualLoadingKeys] = useState<Record<string, boolean>>({});
  const [visualErrors, setVisualErrors] = useState<Record<string, string>>({});
  const [focusedProfilePoint, setFocusedProfilePoint] = useState<ProfilePoint | null>(null);

  const authUrl = apiUrl('/api/auth/strava/login');
  const todayWeekKey = startOfWeekKey(new Date());

  const sortedActivities = useMemo(
    () => [...activities].sort((left, right) => new Date(right.start_date).getTime() - new Date(left.start_date).getTime()),
    [activities],
  );

  const latestSyncedActivity = me ? sortedActivities[0] ?? null : latestActivity;
  const latestWeekKey = latestSyncedActivity ? startOfWeekKey(new Date(latestSyncedActivity.start_date)) : todayWeekKey;
  const weekOptions = useMemo(
    () => buildWeekOptions(sortedActivities, [todayWeekKey, latestWeekKey, selectedWeekKey]),
    [latestWeekKey, selectedWeekKey, sortedActivities, todayWeekKey],
  );
  const selectedWeekStart = useMemo(() => parseLocalDateKey(selectedWeekKey), [selectedWeekKey]);
  const weekDays = useMemo(() => Array.from({ length: 7 }, (_, index) => addDays(selectedWeekStart, index)), [selectedWeekStart]);
  const weekActivities = useMemo(
    () => sortedActivities.filter((activity) => startOfWeekKey(new Date(activity.start_date)) === selectedWeekKey),
    [selectedWeekKey, sortedActivities],
  );
  const weekSummary = useMemo(() => summarizeWeek(weekActivities), [weekActivities]);
  const selectedActivity =
    sortedActivities.find((activity) => activity.id === selectedActivityId) ??
    weekActivities.find((activity) => activity.id === selectedActivityId) ??
    null;
  const selectedActivityVisual = selectedActivity ? activityVisuals[activityResultKey(selectedActivity)] ?? null : null;
  const latestActivityVisual = latestSyncedActivity ? activityVisuals[activityResultKey(latestSyncedActivity)] ?? null : null;
  const selectedVisualKey = selectedActivity ? activityResultKey(selectedActivity) : null;
  const latestVisualKey = latestSyncedActivity ? activityResultKey(latestSyncedActivity) : null;
  const selectedWeekIndex = weekOptions.findIndex((option) => option.key === selectedWeekKey);
  const newerWeek = selectedWeekIndex > 0 ? weekOptions[selectedWeekIndex - 1] : null;
  const olderWeek = selectedWeekIndex >= 0 && selectedWeekIndex < weekOptions.length - 1 ? weekOptions[selectedWeekIndex + 1] : null;

  const weekActivitiesByDay = useMemo(() => {
    const grouped = new Map<string, Activity[]>();

    for (const activity of weekActivities) {
      const key = localDateKey(new Date(activity.start_date));
      const current = grouped.get(key) ?? [];
      current.push(activity);
      grouped.set(key, current);
    }

    return grouped;
  }, [weekActivities]);

  const loadLatestActivity = async (): Promise<void> => {
    const payload = await apiRequest<LatestActivityResponse>('/api/public/latest-activity');
    setLatestActivity(payload.activity);
  };

  const loadDashboard = async (): Promise<void> => {
    const [mePayload, activitiesPayload] = await Promise.all([
      apiRequest<MeResponse>('/api/me'),
      apiRequest<{ activities: Activity[] }>(`/api/activities?limit=${activityFetchLimit}`),
    ]);

    const nextActivities = [...activitiesPayload.activities].sort(
      (left, right) => new Date(right.start_date).getTime() - new Date(left.start_date).getTime(),
    );
    const nextLatest = nextActivities[0] ?? null;

    setMe(mePayload);
    setActivities(nextActivities);
    setLatestActivity(nextLatest);
    setSelectedWeekKey((current) => {
      if (!nextLatest) {
        return current;
      }
      return current === todayWeekKey ? startOfWeekKey(new Date(nextLatest.start_date)) : current;
    });
    setSelectedActivityId((current) => current ?? nextLatest?.id ?? null);
  };

  const loadActivityVisual = async (activity: Activity): Promise<void> => {
    const key = activityResultKey(activity);
    if (activityVisuals[key] || visualLoadingKeys[key]) {
      return;
    }

    setVisualLoadingKeys((current) => ({ ...current, [key]: true }));
    setVisualErrors((current) => ({ ...current, [key]: '' }));

    try {
      const payload = await apiRequest<ActivityVisualResponse>(`/api/activities/${activity.id}/visual?athlete_id=${activity.athlete_id}`);
      setActivityVisuals((current) => ({ ...current, [key]: payload }));
    } catch (error) {
      setVisualErrors((current) => ({ ...current, [key]: (error as Error).message }));
    } finally {
      setVisualLoadingKeys((current) => ({ ...current, [key]: false }));
    }
  };

  const initialize = async (): Promise<void> => {
    setLoading(true);
    setErrorMessage('');

    try {
      await loadDashboard();
    } catch (error) {
      const message = (error as Error).message;
      if (message !== 'unauthorized') {
        setErrorMessage(message);
      }
      setMe(null);
      setActivities([]);
      setSelectedActivityId(null);
      setSelectedWeekKey(todayWeekKey);
      await loadLatestActivity().catch((latestError) => {
        setErrorMessage((latestError as Error).message);
      });
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    initialize().catch((error) => {
      setErrorMessage((error as Error).message);
      setLoading(false);
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (weekActivities.length === 0) {
      if (selectedActivity && startOfWeekKey(new Date(selectedActivity.start_date)) !== selectedWeekKey) {
        setSelectedActivityId(null);
      }
      return;
    }

    const selectionStillVisible = weekActivities.some((activity) => activity.id === selectedActivityId);
    if (!selectionStillVisible) {
      setSelectedActivityId(weekActivities[0].id);
    }
  }, [selectedActivity, selectedActivityId, selectedWeekKey, weekActivities]);

  useEffect(() => {
    setFocusedProfilePoint(null);
  }, [selectedActivityId]);

  useEffect(() => {
    if (!me) {
      return;
    }

    const targets = [latestSyncedActivity, selectedActivity].filter((activity): activity is Activity => activity !== null);
    for (const activity of targets) {
      void loadActivityVisual(activity);
    }
  }, [latestSyncedActivity, me, selectedActivity]);

  const handleSync = async (): Promise<void> => {
    setSyncing(true);
    setErrorMessage('');
    setStatusMessage('');

    try {
      const response = await apiRequest<SyncResponse>('/api/activities/sync', {
        method: 'POST',
        body: JSON.stringify({ after_days: 365 }),
      });
      setStatusMessage(`Synced ${response.fetched_activities} activities across ${response.pages_fetched} page(s).`);
      await loadDashboard();
    } catch (error) {
      setErrorMessage((error as Error).message);
    } finally {
      setSyncing(false);
    }
  };

  const handleSyncRoutes = async (): Promise<void> => {
    setSyncingRoutes(true);
    setErrorMessage('');
    setStatusMessage('');

    try {
      const response = await apiRequest<RouteSyncResponse>('/api/routes/sync', {
        method: 'POST',
      });
      const skipped = response.skipped_routes > 0 ? ` Skipped ${response.skipped_routes}.` : '';
      setStatusMessage(`Synced ${response.synced_routes} routes.${skipped}`);
    } catch (error) {
      setErrorMessage((error as Error).message);
    } finally {
      setSyncingRoutes(false);
    }
  };

  const handleLogout = async (): Promise<void> => {
    setErrorMessage('');

    try {
      await apiRequest<{ status: string }>('/api/auth/logout', { method: 'POST' });
      setMe(null);
      setActivities([]);
      setSelectedActivityId(null);
      setSelectedWeekKey(todayWeekKey);
      setActivityVisuals({});
      setVisualErrors({});
      setVisualLoadingKeys({});
      await loadLatestActivity();
      setStatusMessage('Logged out.');
    } catch (error) {
      setErrorMessage((error as Error).message);
    }
  };

  const selectWeek = (weekKey: string) => {
    setSelectedWeekKey(weekKey);
  };

  const selectActivity = (activity: Activity) => {
    setSelectedWeekKey(startOfWeekKey(new Date(activity.start_date)));
    setSelectedActivityId(activity.id);
  };

  const renderLatestActivity = (showMedia: boolean) => {
    if (!latestSyncedActivity) {
      return null;
    }

    const polyline = latestSyncedActivity.map_polyline || latestSyncedActivity.summary_polyline || '';
    const hasPhoto = showMedia && Boolean(latestActivityVisual?.primary_photo_url);
    const latestVisualLoading = latestVisualKey ? visualLoadingKeys[latestVisualKey] : false;
    const latestVisualError = latestVisualKey ? visualErrors[latestVisualKey] : '';

    return (
      <section className="panel latest-activity-panel">
        <div className="section-head">
          <div>
            <p className="kicker">Latest Activity</p>
            <h2>{latestSyncedActivity.name}</h2>
            <p className="subtle">
              {latestSyncedActivity.type} · {formatActivityDateTime(latestSyncedActivity.start_date)}
            </p>
          </div>
          <div className="pill-row">
            <span className="route-pill strong">{formatDistance(latestSyncedActivity.distance_meters)}</span>
            <span className="route-pill">{formatDuration(latestSyncedActivity.moving_time_seconds)}</span>
            <span className="route-pill">{Math.round(latestSyncedActivity.total_elevation_gain)} m climb</span>
            <span className="route-pill">{formatPace(latestSyncedActivity.distance_meters, latestSyncedActivity.moving_time_seconds)}</span>
          </div>
        </div>

        {latestVisualError && <p className="message error">{latestVisualError}</p>}

        <div className={`latest-activity-layout ${hasPhoto ? 'has-photo' : 'map-only'}`}>
          <div className="route-map-shell latest-map-shell">
            <RouteMap polyline={polyline} title={latestSyncedActivity.name} />
          </div>

          {hasPhoto && (
            <article className="latest-photo-card">
              <img
                className="detail-media-image"
                src={latestActivityVisual?.primary_photo_url}
                alt={latestSyncedActivity.name}
              />
              <div className="latest-photo-copy">
                <p className="route-visual-label">Latest Media</p>
                <strong>
                  {latestActivityVisual?.photo_count
                    ? `${latestActivityVisual.photo_count} photo${latestActivityVisual.photo_count === 1 ? '' : 's'}`
                    : 'Primary photo'}
                </strong>
                <p className="subtle">
                  The latest synced activity includes Strava media, so the spotlight keeps both the route context and the photo visible.
                </p>
              </div>
            </article>
          )}
        </div>

        {showMedia && latestVisualLoading && !latestActivityVisual && <p className="subtle">Loading Strava media...</p>}
        {latestActivityVisual?.description && <p className="activity-note">{latestActivityVisual.description}</p>}
      </section>
    );
  };

  const renderSelectedActivityDetail = () => {
    if (!me) {
      return null;
    }

    if (!selectedActivity) {
      return (
        <section className="panel activity-detail-panel activity-detail-empty">
          <p className="kicker">Activity Detail</p>
          <h2>No activity selected for this week</h2>
          <p className="subtle">Choose a week with activity or click any item in the calendar to load the map, elevation profile, stats, and photos.</p>
        </section>
      );
    }

    const polyline = selectedActivity.map_polyline || selectedActivity.summary_polyline || '';
    const chartPoints = selectedActivityVisual?.stream_points ?? [];
    const selectedVisualLoading = selectedVisualKey ? visualLoadingKeys[selectedVisualKey] : false;
    const selectedVisualError = selectedVisualKey ? visualErrors[selectedVisualKey] : '';

    return (
      <section className="panel activity-detail-panel">
        <div className="section-head">
          <div>
            <p className="kicker">Activity Detail</p>
            <h2>{selectedActivity.name}</h2>
            <p className="subtle">
              {selectedActivity.type} · {formatActivityDateTime(selectedActivity.start_date)}
            </p>
          </div>
          <div className="pill-row">
            <span className="route-pill strong">{formatDistance(selectedActivity.distance_meters)}</span>
            <span className="route-pill">{formatDuration(selectedActivity.moving_time_seconds)}</span>
            <span className="route-pill">{Math.round(selectedActivity.total_elevation_gain)} m climb</span>
            <span className="route-pill">{formatPace(selectedActivity.distance_meters, selectedActivity.moving_time_seconds)}</span>
          </div>
        </div>

        {selectedVisualError && <p className="message error">{selectedVisualError}</p>}

        <div className="route-map-shell detail-map-shell">
          <RouteMap
            polyline={polyline}
            title={selectedActivity.name}
            streamPoints={chartPoints}
            onFocusPointChange={setFocusedProfilePoint}
            focusPoint={
              focusedProfilePoint?.lat != null && focusedProfilePoint?.lng != null
                ? { lat: focusedProfilePoint.lat, lng: focusedProfilePoint.lng }
                : null
            }
          />
        </div>

        {selectedVisualLoading && !selectedActivityVisual && <p className="subtle">Loading activity visuals and elevation profile...</p>}

        <ElevationProfile
          points={chartPoints}
          title={selectedActivity.name}
          focusedPoint={focusedProfilePoint}
          onFocusPointChange={setFocusedProfilePoint}
        />

        <div className="detail-support-grid">
          <article className="detail-stats-panel">
            <div className="detail-stats-grid">
              <div className="detail-stat-card">
                <p className="route-visual-label">Distance</p>
                <strong>{formatDistance(selectedActivity.distance_meters)}</strong>
              </div>
              <div className="detail-stat-card">
                <p className="route-visual-label">Moving Time</p>
                <strong>{formatDuration(selectedActivity.moving_time_seconds)}</strong>
              </div>
              <div className="detail-stat-card">
                <p className="route-visual-label">Elevation</p>
                <strong>{Math.round(selectedActivity.total_elevation_gain)} m</strong>
              </div>
              <div className="detail-stat-card">
                <p className="route-visual-label">Pace</p>
                <strong>{formatPace(selectedActivity.distance_meters, selectedActivity.moving_time_seconds)}</strong>
              </div>
              <div className="detail-stat-card">
                <p className="route-visual-label">Average Speed</p>
                <strong>{formatSpeed(selectedActivity.average_speed)}</strong>
              </div>
              <div className="detail-stat-card">
                <p className="route-visual-label">Max Speed</p>
                <strong>{formatSpeed(selectedActivity.max_speed)}</strong>
              </div>
            </div>

            {selectedActivityVisual?.description ? (
              <div className="detail-description-card">
                <p className="route-visual-label">Description</p>
                <p className="activity-note">{selectedActivityVisual.description}</p>
              </div>
            ) : (
              <div className="detail-description-card">
                <p className="route-visual-label">Recorded</p>
                <strong>{formatActivityDateTime(selectedActivity.start_date)}</strong>
                <p className="subtle">{selectedActivity.timezone || 'Timezone unavailable'}</p>
              </div>
            )}
          </article>

          <article className="detail-photo-panel">
            {selectedActivityVisual?.primary_photo_url ? (
              <>
                <img
                  className="detail-media-image"
                  src={selectedActivityVisual.primary_photo_url}
                  alt={selectedActivity.name}
                />
                <div className="detail-photo-copy">
                  <p className="route-visual-label">Photos</p>
                  <strong>
                    {selectedActivityVisual.photo_count} photo{selectedActivityVisual.photo_count === 1 ? '' : 's'}
                  </strong>
                  <p className="subtle">Strava exposes the activity’s primary image here along with the elevation stream.</p>
                </div>
              </>
            ) : (
              <div className="detail-photo-empty">
                <p className="route-visual-label">Photos</p>
                <strong>No Strava photo available</strong>
                <p className="subtle">This activity still keeps the full-width route map and elevation profile above, even when no media was attached.</p>
              </div>
            )}
          </article>
        </div>
      </section>
    );
  };

  const renderWeeklyCalendar = () => (
    <section className="panel calendar-panel">
      <div className="calendar-head">
        <div>
          <p className="kicker">Weekly Calendar</p>
          <h2>{formatWeekLabel(selectedWeekStart)}</h2>
          <p className="subtle">Every synced activity for the selected week appears here. Pick any item to drive the detail view above.</p>
        </div>

        <div className="calendar-filters">
          <div className="calendar-nav">
            <button className="btn ghost" type="button" onClick={() => newerWeek && selectWeek(newerWeek.key)} disabled={!newerWeek}>
              Newer
            </button>
            <button className="btn ghost" type="button" onClick={() => olderWeek && selectWeek(olderWeek.key)} disabled={!olderWeek}>
              Older
            </button>
          </div>

          <div className="calendar-filter-pills">
            <button
              className={`filter-pill ${selectedWeekKey === latestWeekKey ? 'active' : ''}`}
              type="button"
              onClick={() => selectWeek(latestWeekKey)}
            >
              Latest week
            </button>
            <button
              className={`filter-pill ${selectedWeekKey === todayWeekKey ? 'active' : ''}`}
              type="button"
              onClick={() => selectWeek(todayWeekKey)}
            >
              This week
            </button>
          </div>

          <label className="calendar-select">
            <span>Week</span>
            <select value={selectedWeekKey} onChange={(event) => selectWeek(event.target.value)}>
              {weekOptions.map((option) => (
                <option key={option.key} value={option.key}>
                  {option.label}
                </option>
              ))}
            </select>
          </label>
        </div>
      </div>

      <div className="weekly-stat-grid">
        <article className="weekly-stat-card">
          <p className="route-visual-label">Activities</p>
          <h3>{weekSummary.totalActivities}</h3>
          <span>{weekSummary.activeDays} active day(s)</span>
        </article>
        <article className="weekly-stat-card">
          <p className="route-visual-label">Distance</p>
          <h3>{weekSummary.totalDistanceKm.toFixed(1)} km</h3>
          <span>Across the selected week</span>
        </article>
        <article className="weekly-stat-card">
          <p className="route-visual-label">Moving Time</p>
          <h3>{weekSummary.totalMovingHours.toFixed(1)} h</h3>
          <span>Total weekly training time</span>
        </article>
        <article className="weekly-stat-card">
          <p className="route-visual-label">Elevation</p>
          <h3>{Math.round(weekSummary.totalElevationGain)} m</h3>
          <span>Total weekly climb</span>
        </article>
      </div>

      <div className="calendar-grid">
        {weekDays.map((day) => {
          const dayKey = localDateKey(day);
          const dayActivities = weekActivitiesByDay.get(dayKey) ?? [];
          const isToday = dayKey === localDateKey(new Date());
          const isSelectedDay = selectedActivity ? dayKey === localDateKey(new Date(selectedActivity.start_date)) : false;

          return (
            <section
              key={dayKey}
              className={`calendar-day ${dayActivities.length > 0 ? 'has-activity' : ''} ${isToday ? 'today' : ''} ${
                isSelectedDay ? 'selected' : ''
              }`}
            >
              <div className="calendar-day-head">
                <div>
                  <p className="calendar-weekday">{day.toLocaleDateString(undefined, { weekday: 'short' })}</p>
                  <strong>{day.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })}</strong>
                </div>
                <span className="calendar-day-count">
                  {dayActivities.length === 0 ? 'Rest' : `${dayActivities.length} item${dayActivities.length === 1 ? '' : 's'}`}
                </span>
              </div>

              <div className="calendar-day-body">
                {dayActivities.length === 0 ? (
                  <p className="subtle">No synced activity.</p>
                ) : (
                  dayActivities.map((activity) => {
                    const meta = activityTypeMeta(activity.type);
                    return (
                      <button
                        key={activity.id}
                        type="button"
                        className={`calendar-activity ${selectedActivityId === activity.id ? 'active' : ''}`}
                        onClick={() => selectActivity(activity)}
                      >
                        <span className={`activity-icon activity-icon-${meta.tone}`} aria-hidden="true">
                          {meta.short}
                        </span>
                        <span className="calendar-activity-copy">
                          <strong>{activity.name}</strong>
                          <span>
                            {activity.type} · {formatActivityTime(activity.start_date)}
                          </span>
                        </span>
                      </button>
                    );
                  })
                )}
              </div>
            </section>
          );
        })}
      </div>
    </section>
  );

  if (loading) {
    return (
      <main className="app-shell">
        <section className="panel loading-panel">Loading dashboard...</section>
      </main>
    );
  }

  if (!me) {
    return (
      <main className="app-shell">
        <section className="panel hero-panel hero-shell">
          <div className="hero-copy-block">
            <p className="kicker">React + Go + Strava</p>
            <h1>StrideScope</h1>
            <p className="hero-copy">
              Weekly Strava planning, full-width route detail, and a cleaner activity-first dashboard built on your synced cache.
            </p>
          </div>
          <div className="hero-actions">
            <a className="btn primary" href={authUrl}>
              Connect with Strava
            </a>
          </div>
        </section>

        {renderLatestActivity(false)}

        {(statusMessage || errorMessage) && (
          <section className="panel messages">
            {statusMessage && <p className="message ok">{statusMessage}</p>}
            {errorMessage && <p className="message error">{errorMessage}</p>}
          </section>
        )}
      </main>
    );
  }

  return (
    <main className="app-shell">
      <header className="topbar panel">
        <div className="topbar-brand">
          <p className="kicker">Strava Weekly Dashboard</p>
          <h1>StrideScope</h1>
          <p className="subtle">Maps, elevation, photos, and weekly planning in one flow.</p>
        </div>

        <div className="action-dock">
          <button className="btn secondary" onClick={handleSync} disabled={syncing}>
            {syncing ? 'Syncing activities...' : 'Sync Activities'}
          </button>
          <button className="btn secondary" onClick={handleSyncRoutes} disabled={syncingRoutes}>
            {syncingRoutes ? 'Syncing routes...' : 'Sync Routes'}
          </button>
          <button className="btn ghost" onClick={handleLogout}>
            Log Out
          </button>
        </div>
      </header>

      {(statusMessage || errorMessage) && (
        <section className="panel messages">
          {statusMessage && <p className="message ok">{statusMessage}</p>}
          {errorMessage && <p className="message error">{errorMessage}</p>}
        </section>
      )}

      {renderLatestActivity(true)}
      {renderSelectedActivityDetail()}
      {renderWeeklyCalendar()}
    </main>
  );
}
