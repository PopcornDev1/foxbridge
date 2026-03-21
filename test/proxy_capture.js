// CDP proxy that logs all messages between Puppeteer and Chrome
const WebSocket = require('ws');
const http = require('http');
const puppeteer = require('puppeteer');

async function main() {
  // 1. Launch Chrome with CDP
  const browser = await puppeteer.launch({ headless: true, args: ['--no-sandbox'] });
  const chromeWsUrl = browser.wsEndpoint();
  console.log('Chrome:', chromeWsUrl);
  await browser.disconnect(); // Disconnect Puppeteer so we can proxy

  // 2. Create a proxy WebSocket server
  const proxyPort = 9333;
  const wss = new WebSocket.Server({ port: proxyPort });
  const messages = [];

  wss.on('connection', (clientWs) => {
    console.log('Client connected to proxy');
    const chromeWs = new WebSocket(chromeWsUrl);

    let chromeReady = false;
    const queued = [];
    chromeWs.on('open', () => { chromeReady = true; queued.forEach(d => chromeWs.send(d)); queued.length = 0; });

    // Client → Chrome
    clientWs.on('message', (data) => {
      const msg = JSON.parse(data.toString());
      messages.push({dir: '→', id: msg.id, method: msg.method, sessionId: msg.sessionId || '', params: msg.params});
      if (chromeReady) chromeWs.send(data.toString());
      else queued.push(data.toString());
    });

    // Chrome → Client
    chromeWs.on('message', (data) => {
      const msg = JSON.parse(data.toString());
      if (msg.id) {
        messages.push({dir: '←', id: msg.id, result: msg.result ? JSON.stringify(msg.result).substring(0, 150) : undefined, error: msg.error, sessionId: msg.sessionId || ''});
      } else if (msg.method) {
        messages.push({dir: '←EVT', method: msg.method, sessionId: msg.sessionId || '', params: JSON.stringify(msg.params).substring(0, 200)});
      }
      clientWs.send(data.toString());
    });

    chromeWs.on('close', () => clientWs.close());
    clientWs.on('close', () => chromeWs.close());
  });

  console.log(`Proxy on ws://127.0.0.1:${proxyPort}`);

  // 3. Connect Puppeteer through the proxy
  const proxiedBrowser = await puppeteer.connect({
    browserWSEndpoint: `ws://127.0.0.1:${proxyPort}`,
    defaultViewport: null,
  });
  console.log('Puppeteer connected via proxy\n');

  const page = await proxiedBrowser.newPage();
  console.log('✓ Page created\n');

  // Print the first 40 messages
  console.log('=== CDP Protocol Trace (first 40 messages) ===\n');
  messages.slice(0, 40).forEach((m, i) => {
    const sid = m.sessionId ? `sess=${m.sessionId.substring(0,8)}` : '';
    if (m.dir === '→') {
      console.log(`${i}: → #${m.id} ${m.method} ${sid}`);
      if (m.method.includes('createTarget') || m.method.includes('setAutoAttach') || m.method.includes('Debugger'))
        console.log(`     params: ${JSON.stringify(m.params)}`);
    } else if (m.dir === '←') {
      const val = m.error ? `ERROR: ${JSON.stringify(m.error)}` : m.result;
      console.log(`${i}: ← #${m.id} ${sid} ${val}`);
    } else {
      console.log(`${i}: ←EVT ${m.method} ${sid}`);
      if (m.method.includes('attachedToTarget') || m.method.includes('executionContext'))
        console.log(`     ${m.params}`);
    }
  });

  await proxiedBrowser.close();
  wss.close();
}

main().catch(e => { console.error(e); process.exit(1); });
