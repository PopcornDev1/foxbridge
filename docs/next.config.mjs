import nextra from 'nextra'

const withNextra = nextra({
  contentDirBasePath: '/docs'
})

export default withNextra({
  output: 'export',
  images: { unoptimized: true },
})
