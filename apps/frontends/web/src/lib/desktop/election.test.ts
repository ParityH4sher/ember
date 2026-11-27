import { test } from 'node:test';
import assert from 'node:assert/strict';
import { elect, primaryClaim, type Claim } from './election.ts';

const claim = (id: string, since: number, primary = true, verified = true): Claim =>
  ({ id, since, primary, verified });

test('without the API the single window is primary', () => {
  assert.deepEqual(primaryClaim(null), { primary: true, verified: false });
  assert.equal(elect([claim('a', 1, true, false)]), 'a');
});

test('a single display is primary', () => {
  const one = { screens: [{}], currentScreen: { isPrimary: false } };
  assert.deepEqual(primaryClaim(one), { primary: true, verified: true });
});

test('with several displays only the primary one claims', () => {
  const screens = [{}, {}];
  assert.equal(primaryClaim({ screens, currentScreen: { isPrimary: true } }).primary, true);
  assert.equal(primaryClaim({ screens, currentScreen: { isPrimary: false } }).primary, false);
  assert.equal(elect([claim('side', 1, false), claim('main', 2, true)]), 'main');
});

test('nobody claims, nobody shows', () => {
  assert.equal(elect([claim('a', 1, false), claim('b', 2, false)]), null);
  assert.equal(elect([]), null);
});

test('exactly one winner when several windows claim', () => {
  const claims = [claim('c', 3), claim('a', 2), claim('b', 2)];
  assert.equal(elect(claims), 'a');
  assert.equal(elect([...claims].reverse()), 'a');
});

test('an API-verified claim beats an older fallback claim', () => {
  assert.equal(elect([claim('old', 1, true, false), claim('new', 9, true, true)]), 'new');
});
