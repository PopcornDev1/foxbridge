import { Layout, Navbar } from 'nextra-theme-docs'
import { Head } from 'nextra/components'
import { getPageMap } from 'nextra/page-map'
import { Analytics } from '@vercel/analytics/react'
import { SpeedInsights } from '@vercel/speed-insights/next'
import 'nextra-theme-docs/style.css'

export const metadata = {
  title: {
    default: 'Foxbridge — CDP-to-Firefox Protocol Proxy',
    template: '%s | Foxbridge',
  },
  description: 'Foxbridge translates Chrome DevTools Protocol (CDP) to Firefox via Juggler and WebDriver BiDi. Use Puppeteer, OpenClaw, and any CDP tool with Firefox and Camoufox.',
  icons: {
    icon: '/FoxbridgeLogo.png',
    apple: '/FoxbridgeLogo.png',
  },
  metadataBase: new URL('https://foxbridge.vulpineos.com'),
  openGraph: {
    title: 'Foxbridge — CDP-to-Firefox Protocol Proxy',
    description: 'Translate Chrome DevTools Protocol to Firefox. Use Puppeteer and OpenClaw with Camoufox for undetectable browser automation.',
    url: 'https://foxbridge.vulpineos.com',
    siteName: 'Foxbridge',
    images: [
      {
        url: '/FoxbridgeBanner.jpg',
        width: 1200,
        height: 630,
        alt: 'Foxbridge — CDP-to-Firefox Protocol Proxy',
      },
    ],
    locale: 'en_US',
    type: 'website',
  },
  twitter: {
    card: 'summary_large_image',
    title: 'Foxbridge — CDP-to-Firefox Protocol Proxy',
    description: 'Translate Chrome DevTools Protocol to Firefox. Use Puppeteer and OpenClaw with Camoufox.',
    images: ['/FoxbridgeBanner.jpg'],
  },
  alternates: {
    canonical: 'https://foxbridge.vulpineos.com',
  },
  keywords: ['CDP', 'Firefox', 'Puppeteer', 'OpenClaw', 'Camoufox', 'Chrome DevTools Protocol', 'WebDriver BiDi', 'Juggler', 'browser automation', 'anti-detect browser', 'foxbridge', 'CDP proxy', 'Firefox CDP', 'VulpineOS'],
}

const logo = (
  <span style={{ display: 'flex', alignItems: 'center', gap: '8px', fontWeight: 800, fontSize: '1.1rem' }}>
    <img src="/FoxbridgeLogo.png" alt="Foxbridge" width={28} height={28} style={{ borderRadius: '50%' }} />
    <span>Foxbridge</span>
  </span>
)

const navbar = (
  <Navbar
    logo={logo}
    projectLink="https://github.com/VulpineOS/foxbridge"
  />
)

export default async function RootLayout({ children }) {
  return (
    <html lang="en" dir="ltr" suppressHydrationWarning>
      <Head>
        <meta name="theme-color" content="#F97316" />
        <link rel="icon" href="/FoxbridgeLogo.png" />
        <script
          type="application/ld+json"
          dangerouslySetInnerHTML={{
            __html: JSON.stringify({
              '@context': 'https://schema.org',
              '@type': 'SoftwareApplication',
              name: 'Foxbridge',
              description: 'CDP-to-Firefox Protocol Proxy — translates Chrome DevTools Protocol to Firefox via Juggler and WebDriver BiDi',
              url: 'https://foxbridge.vulpineos.com',
              applicationCategory: 'DeveloperApplication',
              operatingSystem: 'macOS, Linux',
              programmingLanguage: 'Go',
              codeRepository: 'https://github.com/VulpineOS/foxbridge',
              license: 'https://opensource.org/licenses/MIT',
              author: {
                '@type': 'Person',
                name: 'Elliot',
              },
              offers: {
                '@type': 'Offer',
                price: '0',
                priceCurrency: 'USD',
              },
            }),
          }}
        />
        <script
          type="application/ld+json"
          dangerouslySetInnerHTML={{
            __html: JSON.stringify({
              '@context': 'https://schema.org',
              '@type': 'WebSite',
              name: 'Foxbridge Documentation',
              url: 'https://foxbridge.vulpineos.com',
              description: 'Documentation for Foxbridge CDP-to-Firefox protocol proxy',
              publisher: {
                '@type': 'Person',
                name: 'Elliot',
              },
            }),
          }}
        />
      </Head>
      <body>
        <Layout
          navbar={navbar}
          pageMap={await getPageMap()}
          docsRepositoryBase="https://github.com/VulpineOS/foxbridge/tree/main/docs/content"
          footer={<></>}
          sidebar={{ defaultMenuCollapseLevel: 1 }}
        >
          {children}
        </Layout>
        <Analytics />
        <SpeedInsights />
      </body>
    </html>
  )
}
