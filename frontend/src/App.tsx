import { FormEvent, useEffect, useState } from 'react';
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

type Route = {
  id: number;
  athlete_id: number;
  name: string;
  description: string;
  distance_meters: number;
  elevation_gain: number;
  estimated_moving_time: number;
  map_polyline: string;
  summary_polyline: string;
  owner_firstname: string;
  owner_lastname: string;
  owner_profile_medium: string;
  owner_profile: string;
};

type Summary = {
  total_activities: number;
  total_distance_km: number;
  total_moving_hours: number;
  total_elevation_gain: number;
};

type SummaryResponse = {
  days: number;
  summary: Summary;
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

type RouteSearchResponse = {
  query: string;
  routes: Route[];
  activities: Activity[];
};

type LatestActivityResponse = {
  activity: Activity | null;
};

type SearchSelection =
  | { kind: 'activity'; activity: Activity }
  | { kind: 'route'; route: Route };

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

function routeOwner(route: Route): string {
  const fullName = `${route.owner_firstname} ${route.owner_lastname}`.trim();
  return fullName || 'Strava athlete';
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

function routeResultKey(route: Route): string {
  return `route:${route.id}`;
}

function defaultSelectionKey(activities: Activity[], routes: Route[]): string | null {
  if (activities.length > 0) {
    return activityResultKey(activities[0]);
  }
  if (routes.length > 0) {
    return routeResultKey(routes[0]);
  }
  return null;
}

function resolveSelection(key: string | null, activities: Activity[], routes: Route[]): SearchSelection | null {
  if (!key) {
    return null;
  }

  if (key.startsWith('activity:')) {
    const id = Number(key.slice('activity:'.length));
    const activity = activities.find((candidate) => candidate.id === id);
    return activity ? { kind: 'activity', activity } : null;
  }

  if (key.startsWith('route:')) {
    const id = Number(key.slice('route:'.length));
    const route = routes.find((candidate) => candidate.id === id);
    return route ? { kind: 'route', route } : null;
  }

  return null;
}

export default function App() {
  const [me, setMe] = useState<MeResponse | null>(null);
  const [activities, setActivities] = useState<Activity[]>([]);
  const [summary, setSummary] = useState<SummaryResponse | null>(null);
  const [days, setDays] = useState<number>(90);
  const [loading, setLoading] = useState<boolean>(true);
  const [syncing, setSyncing] = useState<boolean>(false);
  const [syncingRoutes, setSyncingRoutes] = useState<boolean>(false);
  const [searchLoading, setSearchLoading] = useState<boolean>(false);
  const [statusMessage, setStatusMessage] = useState<string>('');
  const [errorMessage, setErrorMessage] = useState<string>('');
  const [searchQuery, setSearchQuery] = useState<string>('');
  const [activityResults, setActivityResults] = useState<Activity[]>([]);
  const [routeResults, setRouteResults] = useState<Route[]>([]);
  const [selectedSearchKey, setSelectedSearchKey] = useState<string | null>(null);
  const [latestActivity, setLatestActivity] = useState<Activity | null>(null);

  const authUrl = apiUrl('/api/auth/strava/login');

  const loadDashboard = async (daysWindow: number): Promise<void> => {
    const [mePayload, summaryPayload, activitiesPayload] = await Promise.all([
      apiRequest<MeResponse>('/api/me'),
      apiRequest<SummaryResponse>(`/api/analytics/summary?days=${daysWindow}`),
      apiRequest<{ activities: Activity[] }>('/api/activities?limit=80'),
    ]);

    setMe(mePayload);
    setSummary(summaryPayload);
    setActivities(activitiesPayload.activities);
    setLatestActivity((current) => activitiesPayload.activities[0] ?? current);
  };

  const loadSearch = async (query: string): Promise<RouteSearchResponse> => {
    setSearchLoading(true);

    try {
      const params = new URLSearchParams({ limit: '12' });
      const trimmedQuery = query.trim();
      if (trimmedQuery) {
        params.set('q', trimmedQuery);
      }

      const payload = await apiRequest<RouteSearchResponse>(`/api/search?${params.toString()}`);
      setActivityResults(payload.activities);
      setRouteResults(payload.routes);
      setSelectedSearchKey((current) =>
        resolveSelection(current, payload.activities, payload.routes) ? current : defaultSelectionKey(payload.activities, payload.routes),
      );
      return payload;
    } finally {
      setSearchLoading(false);
    }
  };

  const loadLatestActivity = async (): Promise<void> => {
    const payload = await apiRequest<LatestActivityResponse>('/api/public/latest-activity');
    setLatestActivity(payload.activity);
  };

  const syncRoutes = async (showStatusMessage: boolean, query: string): Promise<RouteSearchResponse> => {
    const response = await apiRequest<RouteSyncResponse>('/api/routes/sync', {
      method: 'POST',
    });

    if (showStatusMessage) {
      const skipped = response.skipped_routes > 0 ? ` Skipped ${response.skipped_routes}.` : '';
      setStatusMessage(`Synced ${response.synced_routes} routes.${skipped}`);
    }

    return loadSearch(query);
  };

  const initialize = async (): Promise<void> => {
    setLoading(true);
    setErrorMessage('');

    let initialSearch: RouteSearchResponse = { query: '', activities: [], routes: [] };

    try {
      await Promise.all([
        loadLatestActivity(),
        loadSearch('').then((payload) => {
          initialSearch = payload;
        }),
      ]);
    } catch (error) {
      setErrorMessage((error as Error).message);
    }

    try {
      await loadDashboard(days);
      if (initialSearch.routes.length === 0) {
        const warmedSearch = await syncRoutes(false, '');
        if (warmedSearch.routes.length > 0) {
          setStatusMessage(
            `Synced ${warmedSearch.routes.length} route${warmedSearch.routes.length === 1 ? '' : 's'} into the public catalog.`,
          );
        }
      }
    } catch (error) {
      if ((error as Error).message !== 'unauthorized') {
        setErrorMessage((error as Error).message);
      }
      setMe(null);
      setSummary(null);
      setActivities([]);
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
      await loadDashboard(days);
      await loadSearch(searchQuery);
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
      await syncRoutes(true, searchQuery);
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
      setSummary(null);
      setActivities([]);
      setStatusMessage('Logged out.');
    } catch (error) {
      setErrorMessage((error as Error).message);
    }
  };

  const handleDaysChange = async (value: number): Promise<void> => {
    setDays(value);
    if (!me) {
      return;
    }

    try {
      const payload = await apiRequest<SummaryResponse>(`/api/analytics/summary?days=${value}`);
      setSummary(payload);
    } catch (error) {
      setErrorMessage((error as Error).message);
    }
  };

  const handleSearch = async (event: FormEvent<HTMLFormElement>): Promise<void> => {
    event.preventDefault();
    setErrorMessage('');

    try {
      await loadSearch(searchQuery);
    } catch (error) {
      setErrorMessage((error as Error).message);
    }
  };

  const renderSearchExplorer = (canSyncRoutes: boolean) => {
    const selectedResult = resolveSelection(selectedSearchKey, activityResults, routeResults);
    const selectedRoute = selectedResult?.kind === 'route' ? selectedResult.route : null;
    const selectedActivity = selectedResult?.kind === 'activity' ? selectedResult.activity : null;
    const routePolyline = selectedRoute?.map_polyline || selectedRoute?.summary_polyline || '';
    const activityPolyline = selectedActivity?.map_polyline || selectedActivity?.summary_polyline || '';
    const hasQuery = searchQuery.trim().length > 0;
    const hasAnyResults = activityResults.length > 0 || routeResults.length > 0;

    return (
      <section className="route-shell">
        <section className="panel route-search-panel">
          <div className="route-toolbar">
            <div>
              <p className="kicker">Unified Search</p>
              <h2>Search synced routes and activities</h2>
              <p className="subtle">
                Activities come from your synced Strava feed. Routes are separate saved Strava routes, so it is normal to have activities even
                when the routes list is empty.
              </p>
            </div>
            {canSyncRoutes ? (
              <button className="btn secondary" onClick={handleSyncRoutes} disabled={syncingRoutes}>
                {syncingRoutes ? 'Syncing routes...' : 'Sync Routes'}
              </button>
            ) : (
              <a className="btn ghost" href={authUrl}>
                Log In To Sync More
              </a>
            )}
          </div>

          <form className="route-form" onSubmit={(event) => handleSearch(event).catch(() => undefined)}>
            <input
              type="search"
              value={searchQuery}
              onChange={(event) => setSearchQuery(event.target.value)}
              placeholder="Search activity names, route names, or route descriptions"
              aria-label="Search activities and routes"
            />
            <button className="btn primary" type="submit" disabled={searchLoading}>
              {searchLoading ? 'Searching...' : 'Search'}
            </button>
          </form>

          <div className="route-results">
            {searchLoading ? (
              <p className="subtle">Searching synced activities and routes...</p>
            ) : !hasAnyResults ? (
              <p className="subtle">
                {hasQuery
                  ? 'No synced activities or routes matched that search.'
                  : 'No synced activities are cached yet. Sync activities to search them. Saved routes are a separate Strava list, so route results can still be empty even when activities exist.'}
              </p>
            ) : (
              <>
                {activityResults.length > 0 && (
                  <>
                    <p className="search-section-label">Activities</p>
                    {activityResults.map((activity) => (
                      <button
                        key={activity.id}
                        className={`route-card ${selectedSearchKey === activityResultKey(activity) ? 'active' : ''}`}
                        onClick={() => setSelectedSearchKey(activityResultKey(activity))}
                        type="button"
                      >
                        <span className="route-card-head">
                          <strong>{activity.name}</strong>
                          <span className="search-tag">Activity</span>
                        </span>
                        <span className="route-card-meta">
                          {activity.type} · {new Date(activity.start_date).toLocaleDateString()}
                        </span>
                        <span className="route-card-owner">
                          {formatDistance(activity.distance_meters)} · {formatDuration(activity.moving_time_seconds)}
                        </span>
                      </button>
                    ))}
                  </>
                )}

                {routeResults.length > 0 && (
                  <>
                    <p className="search-section-label">Routes</p>
                    {routeResults.map((route) => (
                      <button
                        key={route.id}
                        className={`route-card ${selectedSearchKey === routeResultKey(route) ? 'active' : ''}`}
                        onClick={() => setSelectedSearchKey(routeResultKey(route))}
                        type="button"
                      >
                        <span className="route-card-head">
                          <strong>{route.name}</strong>
                          <span className="search-tag">Route</span>
                        </span>
                        <span className="route-card-meta">
                          {Math.round(route.elevation_gain)} m climb · {formatDuration(route.estimated_moving_time)}
                        </span>
                        <span className="route-card-owner">{routeOwner(route)}</span>
                      </button>
                    ))}
                  </>
                )}
              </>
            )}
          </div>
        </section>

        <section className="panel route-detail-panel">
          {!selectedResult ? (
            <div className="route-empty">
              <p className="kicker">Search Detail</p>
              <h2>Select an activity or route</h2>
              <p className="subtle">
                Activities come from your synced feed. Routes come only from Strava&apos;s saved routes list, so a blank routes section does
                not mean your activities are missing.
              </p>
            </div>
          ) : selectedActivity ? (
            <>
              <div className="route-detail-head">
                <div>
                  <p className="kicker">Activity Detail</p>
                  <h2>{selectedActivity.name}</h2>
                  <p className="subtle">
                    {selectedActivity.type} · {new Date(selectedActivity.start_date).toLocaleString()}
                  </p>
                </div>
                <div className="route-stat-row">
                  <span className="route-pill strong">{formatDistance(selectedActivity.distance_meters)}</span>
                  <span className="route-pill">{formatDuration(selectedActivity.moving_time_seconds)}</span>
                  <span className="route-pill">{Math.round(selectedActivity.total_elevation_gain)} m climb</span>
                  <span className="route-pill">{formatPace(selectedActivity.distance_meters, selectedActivity.moving_time_seconds)}</span>
                </div>
              </div>

              <div className="route-map-shell">
                <RouteMap polyline={activityPolyline} title={selectedActivity.name} />
              </div>

              <div className="route-visuals">
                <div className="detail-info-card">
                  <div>
                    <p className="route-visual-label">Recorded</p>
                    <strong>{new Date(selectedActivity.start_date).toLocaleString()}</strong>
                    <p className="subtle">{selectedActivity.timezone || 'Timezone unavailable'}</p>
                  </div>
                </div>

                <div className="detail-info-card">
                  <div>
                    <p className="route-visual-label">Speed</p>
                    <strong>{formatSpeed(selectedActivity.average_speed)} average</strong>
                    <p className="subtle">{formatSpeed(selectedActivity.max_speed)} max</p>
                  </div>
                </div>
              </div>
            </>
          ) : selectedRoute ? (
            <>
              <div className="route-detail-head">
                <div>
                  <p className="kicker">Route Detail</p>
                  <h2>{selectedRoute.name}</h2>
                  <p className="subtle">{routeOwner(selectedRoute)}</p>
                </div>
                <div className="route-stat-row">
                  <span className="route-pill strong">{formatDistance(selectedRoute.distance_meters)}</span>
                  <span className="route-pill">{Math.round(selectedRoute.elevation_gain)} m climb</span>
                  <span className="route-pill">{formatDuration(selectedRoute.estimated_moving_time)}</span>
                </div>
              </div>

              <div className="route-map-shell">
                <RouteMap polyline={routePolyline} title={selectedRoute.name} />
              </div>

              {selectedRoute.description && <p className="route-description">{selectedRoute.description}</p>}

              <div className="route-visuals">
                <div className="route-owner-card">
                  {selectedRoute.owner_profile_medium || selectedRoute.owner_profile ? (
                    <img
                      className="route-owner-image"
                      src={selectedRoute.owner_profile_medium || selectedRoute.owner_profile}
                      alt={routeOwner(selectedRoute)}
                    />
                  ) : (
                    <div className="route-owner-fallback">{routeOwner(selectedRoute).slice(0, 1)}</div>
                  )}
                  <div>
                    <p className="route-visual-label">Creator image</p>
                    <strong>{routeOwner(selectedRoute)}</strong>
                  </div>
                </div>

                <div className="route-api-note">
                  <p className="route-visual-label">Pictures</p>
                  <p className="subtle">
                    Strava route endpoints provide route geometry and athlete profile images, but they do not expose a per-route photo gallery.
                  </p>
                </div>
              </div>
            </>
          ) : null}
        </section>
      </section>
    );
  };

  const renderLatestActivity = () => {
    if (!latestActivity) {
      return null;
    }

    const polyline = latestActivity.map_polyline || latestActivity.summary_polyline || '';

    return (
      <section className="panel activity-spotlight">
        <div className="activity-spotlight-head">
          <div>
            <p className="kicker">Latest Activity</p>
            <h2>{latestActivity.name}</h2>
            <p className="subtle">
              {latestActivity.type} · {new Date(latestActivity.start_date).toLocaleString()}
            </p>
          </div>
          <div className="activity-spotlight-stats">
            <span className="route-pill strong">{formatDistance(latestActivity.distance_meters)}</span>
            <span className="route-pill">{formatDuration(latestActivity.moving_time_seconds)}</span>
            <span className="route-pill">{Math.round(latestActivity.total_elevation_gain)} m climb</span>
            <span className="route-pill">{formatPace(latestActivity.distance_meters, latestActivity.moving_time_seconds)}</span>
          </div>
        </div>

        <div className="activity-spotlight-grid">
          <div className="route-map-shell activity-map-shell">
            <RouteMap polyline={polyline} title={latestActivity.name} />
          </div>
          <div className="activity-spotlight-copy">
            <p className="subtle">
              This is the most recent synced Strava activity currently cached by the app. The search panel below now searches both cached
              activities and saved routes.
            </p>
          </div>
        </div>
      </section>
    );
  };

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
        <section className="panel hero-panel">
          <p className="kicker">React + Go + Strava</p>
          <h1>StrideScope</h1>
          <p className="hero-copy">
            Track volume, pace, elevation, and route ideas from your Strava data with a Go API, DynamoDB cache, and public route search.
          </p>
          <a className="btn primary" href={authUrl}>
            Connect with Strava
          </a>
          {errorMessage && <p className="message error">{errorMessage}</p>}
        </section>

        {renderLatestActivity()}
        {renderSearchExplorer(false)}
      </main>
    );
  }

  return (
    <main className="app-shell">
      <header className="topbar panel">
        <div>
          <p className="kicker">Welcome back</p>
          <h1>
            {me.athlete.firstname} {me.athlete.lastname}
          </h1>
          <p className="subtle">
            Token expires: {new Date(me.token_expires_at).toLocaleString()} · {me.athlete.city}, {me.athlete.country}
          </p>
        </div>
        <div className="topbar-actions">
          <button className="btn secondary" onClick={handleSync} disabled={syncing}>
            {syncing ? 'Syncing...' : 'Sync Activities'}
          </button>
          <button className="btn secondary" onClick={handleSyncRoutes} disabled={syncingRoutes}>
            {syncingRoutes ? 'Syncing routes...' : 'Sync Routes'}
          </button>
          <button className="btn ghost" onClick={handleLogout}>
            Log Out
          </button>
        </div>
      </header>

      {renderLatestActivity()}
      {renderSearchExplorer(true)}

      <section className="panel controls">
        <label htmlFor="window">Summary window</label>
        <select
          id="window"
          value={days}
          onChange={(event) => handleDaysChange(Number(event.target.value)).catch(() => undefined)}
        >
          <option value={30}>30 days</option>
          <option value={90}>90 days</option>
          <option value={180}>180 days</option>
          <option value={365}>365 days</option>
        </select>
      </section>

      <section className="stat-grid">
        <article className="panel stat">
          <h2>{summary?.summary.total_activities ?? 0}</h2>
          <p>Activities</p>
        </article>
        <article className="panel stat">
          <h2>{(summary?.summary.total_distance_km ?? 0).toFixed(1)} km</h2>
          <p>Distance</p>
        </article>
        <article className="panel stat">
          <h2>{(summary?.summary.total_moving_hours ?? 0).toFixed(1)} h</h2>
          <p>Moving Time</p>
        </article>
        <article className="panel stat">
          <h2>{Math.round(summary?.summary.total_elevation_gain ?? 0)} m</h2>
          <p>Elevation Gain</p>
        </article>
      </section>

      <section className="panel">
        <h3>Recent Activities</h3>
        {activities.length === 0 ? (
          <p className="subtle">No activities synced yet.</p>
        ) : (
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th>Date</th>
                  <th>Name</th>
                  <th>Type</th>
                  <th>Distance</th>
                  <th>Time</th>
                  <th>Pace</th>
                  <th>Elevation</th>
                </tr>
              </thead>
              <tbody>
                {activities.map((activity) => (
                  <tr key={activity.id}>
                    <td>{new Date(activity.start_date).toLocaleDateString()}</td>
                    <td>{activity.name}</td>
                    <td>{activity.type}</td>
                    <td>{formatDistance(activity.distance_meters)}</td>
                    <td>{formatDuration(activity.moving_time_seconds)}</td>
                    <td>{formatPace(activity.distance_meters, activity.moving_time_seconds)}</td>
                    <td>{Math.round(activity.total_elevation_gain)} m</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      {(statusMessage || errorMessage) && (
        <section className="panel messages">
          {statusMessage && <p className="message ok">{statusMessage}</p>}
          {errorMessage && <p className="message error">{errorMessage}</p>}
        </section>
      )}
    </main>
  );
}
