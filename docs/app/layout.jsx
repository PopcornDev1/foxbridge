import { Footer, Layout, Navbar } from 'nextra-theme-docs'
import { Head } from 'nextra/components'
import { getPageMap } from 'nextra/page-map'
import 'nextra-theme-docs/style.css'

export const metadata = {
  title: 'Foxbridge Docs',
  description: 'CDP-to-Firefox Protocol Proxy',
}

export default async function RootLayout({ children }) {
  return (
    <html lang="en" dir="ltr" suppressHydrationWarning>
      <Head />
      <body>
        <Layout
          navbar={<Navbar logo={<span style={{ fontWeight: 800 }}>🦊 Foxbridge</span>} projectLink="https://github.com/PopcornDev1/foxbridge" />}
          pageMap={await getPageMap()}
          docsRepositoryBase="https://github.com/PopcornDev1/foxbridge/tree/main/docs"
          footer={<Footer>Foxbridge — CDP-to-Firefox Protocol Proxy. Part of <a href="https://vulpineos.com">VulpineOS</a>.</Footer>}
          banner={<a href="https://vulpineos.com" style={{ textAlign: 'center', display: 'block' }}>Part of the VulpineOS ecosystem →</a>}
        >
          {children}
        </Layout>
      </body>
    </html>
  )
}
