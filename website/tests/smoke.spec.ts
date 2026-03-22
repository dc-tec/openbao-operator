import {expect, test} from '@playwright/test';

test('homepage exposes the primary operator journeys', async ({page}) => {
  await page.goto('');

  await expect(
    page.getByRole('heading', {
      name: 'Operate OpenBao like a platform, not a pile of YAML.',
    }),
  ).toBeVisible();
  await expect(page.getByRole('link', {name: 'Open Docs'})).toBeVisible();
  await expect(page.getByRole('link', {name: 'Release Notes', exact: true})).toBeVisible();
  await expect(page.locator('main a[href$="/docs/operate"]').first()).toBeVisible();
  await expect(page.locator('main a[href$="/docs/configure"]').first()).toBeVisible();
  await expect(
    page
      .getByRole('navigation', {name: 'Main'})
      .getByRole('link', {name: 'Contribute', exact: true}),
  ).toBeVisible();
});

test('legacy latest user-guide route redirects into the new IA', async ({page}) => {
  await page.goto('latest/user-guide');
  await expect(page).toHaveURL(/\/openbao-operator\/docs\/get-started$/);
  await expect(
    page.getByRole('heading', {name: 'Deploy OpenBao Operator with a clear first path.'}),
  ).toBeVisible();
});

test('next docs expose the version banner and feedback controls', async ({page}) => {
  await page.goto('docs/next/get-started/deployment-decision-guide');

  await expect(
    page.getByRole('heading', {
      name: 'Choose the deployment path you want to keep operating.',
    }),
  ).toBeVisible();
  await expect(page.getByText('Next release documentation')).toBeVisible();
  await expect(page.getByText('Was this page helpful?')).toBeVisible();
  await expect(page.getByRole('button', {name: 'Yes'})).toBeVisible();
  await expect(page.getByRole('link', {name: 'Needs work'})).toHaveAttribute(
    'href',
    /github\.com\/dc-tec\/openbao-operator\/issues\/new/,
  );
});

test('stable docs expose the current release banner', async ({page}) => {
  await page.goto('docs');

  await expect(
    page.getByRole('heading', {name: 'Choose the section that matches the job in front of you.'}),
  ).toBeVisible();
  await expect(page.getByText('Prerelease documentation')).toBeVisible();
  await expect(page.getByText('Version: 0.1.0-rc.5')).toBeVisible();
});

test('mobile navigation stays usable', async ({page}) => {
  await page.setViewportSize({width: 390, height: 844});
  await page.goto('');

  await page.getByRole('button', {name: 'Toggle navigation bar'}).click();
  const mobileSidebar = page.locator('.navbar-sidebar--show');
  await expect(mobileSidebar).toBeVisible();
  await expect(mobileSidebar.getByRole('button', {name: 'Docs'}).last()).toBeVisible();
  await expect(mobileSidebar.getByRole('link', {name: 'Contribute', exact: true})).toBeVisible();
});
