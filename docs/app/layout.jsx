import { Layout, Navbar } from 'nextra-theme-docs'
import { Head } from 'nextra/components'
import { getPageMap } from 'nextra/page-map'
import 'nextra-theme-docs/style.css'

export const metadata = {
  title: 'Foxbridge',
  description: 'CDP-to-Firefox Protocol Proxy — Run Puppeteer and OpenClaw on Firefox/Camoufox',
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
    projectLink="https://github.com/PopcornDev1/foxbridge"
  />
)

export default async function RootLayout({ children }) {
  return (
    <html lang="en" dir="ltr" suppressHydrationWarning>
      <Head>
        <meta name="theme-color" content="#F97316" />
        <link rel="icon" href="/FoxbridgeLogo.png" />
      </Head>
      <body>
        <Layout
          navbar={navbar}
          pageMap={await getPageMap()}
          docsRepositoryBase="https://github.com/PopcornDev1/foxbridge/tree/main/docs/content"
          footer={<></>}
          sidebar={{ defaultMenuCollapseLevel: 1 }}
        >
          {children}
        </Layout>
      </body>
    </html>
  )
}
