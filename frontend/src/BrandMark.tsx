export default function BrandMark() {
  return (
    <svg viewBox="0 0 112 112" className="brand-mark" aria-hidden="true">
      <defs>
        <linearGradient id="brandGlow" x1="0%" y1="0%" x2="100%" y2="100%">
          <stop offset="0%" stopColor="#ff8a42" />
          <stop offset="100%" stopColor="#fc4c02" />
        </linearGradient>
      </defs>

      <rect x="8" y="8" width="96" height="96" rx="24" fill="url(#brandGlow)" />
      <path
        d="M69 23c-13 0-24 10-24 24s11 24 24 24c5 0 9-1 13-4l2-13c-3 4-8 7-14 7-8 0-15-7-15-15s7-15 15-15c4 0 8 1 11 4l2-12c-4-1-7-1-14-1z"
        fill="rgba(255,255,255,0.14)"
      />
      <path
        d="M20 67 38 46l11 13 9-10 15 18M22 74c8 4 15 4 23 0 8-4 15-4 23 0 8 4 15 4 23 0"
        fill="none"
        stroke="#251f24"
        strokeWidth="3.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <path
        d="M66 77h9l4-5m2 0h5m-15 9 8-1 9 1M72 77l-1-6m10 0-2 10m6-7c0 4-3 7-7 7-4 0-7-3-7-7 0-4 3-7 7-7 4 0 7 3 7 7zm-20 0c0 4-3 7-7 7-4 0-7-3-7-7 0-4 3-7 7-7 4 0 7 3 7 7z"
        fill="none"
        stroke="#251f24"
        strokeWidth="3.2"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}
