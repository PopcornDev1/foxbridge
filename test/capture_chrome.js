// Capture what Puppeteer sends to real Chrome during newPage()
// This gives us the ground truth to match in foxbridge
const puppeteer = require('puppeteer');
const WebSocket = require('ws');

async function main() {
  // Launch real Chrome
  const browser = await puppeteer.launch({
    headless: true,
    args: ['--no-sandbox'],
  });

  const wsUrl = browser.wsEndpoint();
  console.log('Chrome WS:', wsUrl);

  // Connect a spy WebSocket to capture traffic
  const ws = new WebSocket(wsUrl);
  const messages = [];

  ws.on('message', (data) => {
    const msg = JSON.parse(data.toString());
    if (msg.method) {
      messages.push({dir: 'EVENT', method: msg.method, sessionId: msg.sessionId || '', params: JSON.stringify(msg.params).substring(0, 200)});
    }
  });

  await new Promise(r => ws.on('open', r));

  // Now create a page through Puppeteer (which uses the same WS internally)
  console.log('\n=== Creating page via Puppeteer ===');
  const page = await browser.newPage();
  console.log('Page created');

  await page.goto('https://example.com', {waitUntil: 'load'});
  console.log('Navigated');

  const title = await page.title();
  console.log('Title:', title);

  // Wait a moment for all events
  await new Promise(r => setTimeout(r, 1000));

  // Print captured events
  console.log('\n=== Events Chrome sent (first 30) ===');
  messages.slice(0, 30).forEach(m => {
    console.log(`${m.dir} ${m.method} session=${m.sessionId ? m.sessionId.substring(0,8)+'...' : '(browser)'}`);
    if (m.method.includes('executionContext') || m.method.includes('attachedToTarget') || m.method.includes('lifecycle') || m.method.includes('frameNavigated')) {
      console.log(`  params: ${m.params}`);
    }
  });

  await browser.close();
}

main().catch(console.error);
