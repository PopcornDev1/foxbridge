const BASE_URL = 'https://foxbridge.vulpineos.com'

const pages = [
  '', // index
  '/quick-start',
  '/architecture',
  '/cdp-coverage',
  '/event-translation',
  '/session-model',
  '/context-management',
  '/juggler-backend',
  '/bidi-backend',
  '/request-interception',
  '/performance-metrics',
  '/pdf-generation',
  '/input-handling',
  '/cookies-storage',
  '/emulation',
  '/vulpineos',
  '/puppeteer',
  '/openclaw',
  '/how-to',
  '/comparison',
  '/cli-reference',
  '/testing',
  '/contributing',
]

export default function sitemap() {
  return pages.map(page => ({
    url: `${BASE_URL}${page}`,
    lastModified: new Date(),
    changeFrequency: page === '' ? 'weekly' : 'monthly',
    priority: page === '' ? 1.0 : page === '/quick-start' ? 0.9 : 0.7,
  }))
}
