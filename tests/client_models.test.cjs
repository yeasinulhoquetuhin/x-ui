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
    const input = { email: 'client', enable: true, tgId: '-1001234567890', trafficReset: 'custom', trafficResetDays: 15, limitIp: 1, created_at: 12, updated_at: 34 };
    const client = context.Inbound[settings][name].fromJson(input);
    const json = client.toJson();
    assert.equal(json.trafficReset, 'custom');
    assert.equal(json.trafficResetDays, 15);
    assert.equal(json.limitIp, 1);
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
});
