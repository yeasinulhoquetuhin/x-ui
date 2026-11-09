const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const test = require('node:test');
const root = path.resolve(__dirname, '..');
const context = vm.createContext({
  RandomUtil: { randomLowerAndNum: () => 'random', randomUUID: () => 'uuid', randomSeq: () => 'secret', randomShadowsocksPassword: () => 'password' },
});
vm.runInContext(fs.readFileSync(path.join(root, 'web/assets/js/model/inbound.js'), 'utf8') + '\nthis.Inbound = Inbound;', context);

for (const [settings, name] of [['VmessSettings', 'VMESS'], ['VLESSSettings', 'VLESS'], ['TrojanSettings', 'Trojan'], ['ShadowsocksSettings', 'Shadowsocks'], ['HysteriaSettings', 'Hysteria']]) {
  test(`${settings} preserves automatic traffic-reset settings`, () => {
    const input = { email: 'client', enable: true, tgId: '-1001234567890', trafficReset: 'custom', trafficResetDays: 15, showIpLimit: true, limitIp: 1, created_at: 12, updated_at: 34 };
    const client = context.Inbound[settings][name].fromJson(input);
    const json = client.toJson();
    assert.equal(json.trafficReset, 'custom');
    assert.equal(json.trafficResetDays, 15);
    assert.equal(json.limitIp, 1);
    assert.equal(json.showIpLimit, true);
    assert.equal(new context.Inbound[settings][name]().showIpLimit, false);
    assert.equal(json.created_at, 12);
    assert.equal(json.updated_at, 34);
    assert.equal(json.tgId, '-1001234567890');
    assert.equal(new context.Inbound[settings][name]().trafficReset, 'never');
  });
}

test('IP display handles empty and failed snapshots without counting placeholders', async () => {
  const html = fs.readFileSync(path.join(root, 'web/html/modals/inbound_info_modal.html'), 'utf8');
  const helper = html.slice(html.indexOf('function refreshIPs(email)'), html.indexOf('const infoModal ='));
  const ctx = vm.createContext({HttpUtil: {post: async () => ({success:true, obj:[]})}});
  vm.runInContext(helper, ctx);
  assert.equal((await ctx.refreshIPs('client')).array.length, 0);
  ctx.HttpUtil.post = async () => ({success:true, obj:'No IP Record'});
  assert.equal((await ctx.refreshIPs('client')).array.length, 0);
  ctx.HttpUtil.post = async () => ({success:false});
  assert.equal((await ctx.refreshIPs('client')).array.length, 0);
  ctx.HttpUtil.post = async () => ({success:true, obj:['192.0.2.1 (2026-01-01)']});
  assert.equal((await ctx.refreshIPs('client')).array.length, 1);
  ctx.HttpUtil.post = async () => ({success:true, obj:['192.0.2.1', '192.0.2.1', '192.0.2.2']});
  assert.equal((await ctx.refreshIPs('client')).array.length, 2);
  ctx.HttpUtil.post = async () => { throw new Error('API disconnected'); };
  assert.equal((await ctx.refreshIPs('client')).error, true);
});

function ipDetailsContext(post) {
  const html = fs.readFileSync(path.join(root, 'web/html/modals/inbound_info_modal.html'), 'utf8');
  const script = html.slice(html.indexOf('<script>') + 8, html.lastIndexOf('</script>'));
  const ctx = vm.createContext({
    Inbound: class {}, DBInbound: class {},
    HttpUtil: {post}, document: {head: {insertAdjacentHTML() {}}},
    Vue: function(options) {
      const view = Object.assign({}, options.data, options.methods);
      for (const [key, property] of Object.entries(options.computed || {})) {
        Object.defineProperty(view, key, {get: property.get, set: property.set});
      }
      return view;
    },
  });
  vm.runInContext(script + '\nthis.modal = infoModal; this.view = infoModalApp;', ctx);
  ctx.modal.visible = true;
  ctx.modal.clientSettings = {email: 'client@example.com', limitIp: 2};
  ctx.modal.clientIps = '192.0.2.1';
  ctx.modal.clientIpsArray = ['192.0.2.1'];
  return ctx;
}

const settle = () => new Promise(resolve => setImmediate(resolve));

test('clearing IP details does not refresh and cannot be undone by an older response', async () => {
  const calls = [];
  let finishOldRefresh;
  const ctx = ipDetailsContext(url => {
    calls.push(url);
    if (url.includes('/clearClientIps/')) return Promise.resolve({success: true});
    return new Promise(resolve => {finishOldRefresh = resolve;});
  });
  ctx.view.refreshIPs();
  ctx.view.clearClientIps();
  await settle();
  assert.equal(calls.length, 2);
  assert(calls[1].endsWith('/clearClientIps/client%40example.com'));
  assert.equal(ctx.modal.clientIpsArray.length, 0);
  assert.equal(ctx.view.refreshing, false);
  finishOldRefresh({success: true, obj: ['192.0.2.1']});
  await settle();
  assert.equal(ctx.modal.clientIpsArray.length, 0, 'old refresh repopulated cleared records');
  ctx.HttpUtil.post = async url => {
    calls.push(url);
    return {success: true, obj: ['192.0.2.2']};
  };
  ctx.view.refreshIPs();
  await settle();
  assert.equal(ctx.modal.clientIpsArray[0], '192.0.2.2', 'explicit refresh stopped working');
});

test('failed clears preserve the displayed snapshot and cannot erase another client', async () => {
  const ctx = ipDetailsContext(async () => ({success: false}));
  ctx.view.clearClientIps();
  await settle();
  assert.equal(ctx.modal.clientIpsArray[0], '192.0.2.1');
  let finishClear;
  ctx.HttpUtil.post = () => new Promise(resolve => {finishClear = resolve;});
  ctx.view.clearClientIps();
  ctx.modal.clientSettings = {email: 'other-client'};
  ctx.modal.clientIpsArray = ['192.0.2.3'];
  finishClear({success: true});
  await settle();
  assert.equal(ctx.modal.clientIpsArray[0], '192.0.2.3');
});
