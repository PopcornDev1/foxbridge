export default function robots() {
  return {
    rules: [
      {
        userAgent: '*',
        allow: '/',
      },
    ],
    sitemap: 'https://foxbridge.vulpineos.com/sitemap.xml',
  }
}
