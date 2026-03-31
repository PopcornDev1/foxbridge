import { ImageResponse } from 'next/og'

export const runtime = 'edge'

export async function GET(req) {
  const { searchParams } = new URL(req.url)
  const title = searchParams.get('title') || 'Foxbridge'
  const description =
    searchParams.get('description') || 'CDP-to-Firefox Protocol Proxy'

  const logoUrl = new URL('/FoxbridgeLogo.png', req.url).toString()

  return new ImageResponse(
    (
      <div
        style={{
          height: '100%',
          width: '100%',
          display: 'flex',
          flexDirection: 'column',
          justifyContent: 'space-between',
          padding: '60px 80px',
          background: 'linear-gradient(145deg, #1a0c00 0%, #2e1500 40%, #3d1c00 100%)',
          fontFamily: 'system-ui, sans-serif',
        }}
      >
        {/* Top: logo + brand */}
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            gap: '16px',
          }}
        >
          <img
            src={logoUrl}
            width={52}
            height={52}
            style={{ borderRadius: '50%' }}
          />
          <span
            style={{
              fontSize: '26px',
              color: '#fdba74',
              fontWeight: 700,
              letterSpacing: '-0.02em',
            }}
          >
            Foxbridge
          </span>
        </div>

        {/* Center: title + description */}
        <div style={{ display: 'flex', flexDirection: 'column', gap: '20px' }}>
          <div
            style={{
              fontSize: title.length > 40 ? '44px' : '54px',
              fontWeight: 800,
              color: '#ffffff',
              lineHeight: 1.1,
              letterSpacing: '-0.03em',
              maxWidth: '950px',
            }}
          >
            {title}
          </div>
          <div
            style={{
              fontSize: '22px',
              color: '#fed7aa',
              lineHeight: 1.4,
              maxWidth: '800px',
            }}
          >
            {description}
          </div>
        </div>

        {/* Bottom: URL */}
        <div
          style={{
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'space-between',
          }}
        >
          <span style={{ fontSize: '20px', color: '#8a6b3d' }}>
            foxbridge.vulpineos.com
          </span>
          <span style={{ fontSize: '16px', color: '#664d2a' }}>
            CDP-to-Firefox Protocol Proxy
          </span>
        </div>

        {/* Decorative glow */}
        <div
          style={{
            position: 'absolute',
            top: '-100px',
            right: '-50px',
            width: '500px',
            height: '500px',
            background:
              'radial-gradient(circle, rgba(249,115,22,0.2) 0%, transparent 65%)',
          }}
        />
        <div
          style={{
            position: 'absolute',
            bottom: '-80px',
            left: '200px',
            width: '400px',
            height: '400px',
            background:
              'radial-gradient(circle, rgba(253,186,116,0.08) 0%, transparent 65%)',
          }}
        />

        {/* Bottom accent line */}
        <div
          style={{
            position: 'absolute',
            bottom: '0',
            left: '0',
            width: '100%',
            height: '4px',
            background:
              'linear-gradient(90deg, #F97316 0%, #fdba74 50%, #F97316 100%)',
          }}
        />
      </div>
    ),
    {
      width: 1200,
      height: 630,
    },
  )
}
