import {expect, test} from '@playwright/test';

test('homepage exposes the primary operator journeys', async ({page}) => {
  await page.goto('');

  await expect(
    page.getByRole('heading', {
      name: 'Operate OpenBao like a platform, not a pile of YAML.',
    }),
  ).toBeVisible();
  await expect(page.getByRole('link', {name: 'Start with Get Started'}).first()).toBeVisible();
  await expect(page.getByRole('link', {name: 'Release Notes', exact: true})).toBeVisible();
  await expect(page.locator('main a[href$="/docs/operate"]').first()).toBeVisible();
  await expect(page.locator('main a[href$="/docs/configure"]').first()).toBeVisible();
  await expect(page.locator('main a[href$="/docs/reference"]').first()).toBeVisible();
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
    page.getByRole('heading', {name: 'Get started with OpenBao Operator'}),
  ).toBeVisible();
});

test('next docs expose the version banner and feedback controls', async ({page}) => {
  await page.goto('docs/next/get-started/deployment-decision-guide');

  await expect(page.getByRole('heading', {name: 'Choose the deployment path'})).toBeVisible();
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

  await expect(page.getByRole('heading', {name: 'OpenBao Operator documentation'})).toBeVisible();
  await expect(page.getByText('Published release documentation')).toBeVisible();
  await expect(page.getByText('Version: 0.2.0')).toBeVisible();
});

test('architecture section exposes grouped local navigation', async ({page}) => {
  await page.goto('docs/next/architecture');

  await expect(page.getByRole('heading', {name: 'Operator architecture'})).toBeVisible();
  await expect(page.getByText('Read This First', {exact: true})).toBeVisible();
  await expect(page.getByText('Workload Managers', {exact: true})).toBeVisible();
  await expect(page.getByText('Operations Managers', {exact: true})).toBeVisible();
  await expect(page.getByText('Provisioning', {exact: true})).toBeVisible();
  await expect(page.getByText('Supporting Services', {exact: true})).toBeVisible();
  await expect(page.getByText('Lifecycle Flows', {exact: true})).toBeVisible();
});

test('security section exposes grouped local navigation', async ({page}) => {
  await page.goto('docs/next/security');

  await expect(page.getByRole('heading', {name: 'Security documentation'})).toBeVisible();
  await expect(page.getByText('Security Model', {exact: true})).toBeVisible();
  await expect(page.getByText('Platform Controls', {exact: true})).toBeVisible();
  await expect(page.getByText('Workload Protections', {exact: true})).toBeVisible();
  await expect(page.getByText('Tenant Isolation', {exact: true})).toBeVisible();
});

test('configure section exposes grouped local navigation', async ({page}) => {
  await page.goto('docs/next/configure');

  await expect(page.getByRole('heading', {name: 'Cluster configuration'})).toBeVisible();
  await expect(page.getByText('Read This First', {exact: true})).toBeVisible();
  await expect(page.getByText('Cluster Baseline', {exact: true})).toBeVisible();
  await expect(page.getByText('Service Boundary', {exact: true})).toBeVisible();
  await expect(page.getByText('Platform Readiness', {exact: true})).toBeVisible();
});

test('validated deployments expose lane-first local navigation', async ({page}) => {
  await page.goto('docs/next/validated-deployments');

  await expect(
    page.getByRole('heading', {
      name: 'Validated deployment baselines',
    }),
  ).toBeVisible();
  await expect(page.getByText('Cloud Baselines', {exact: true})).toBeVisible();
  await expect(page.getByText('Local Baselines', {exact: true})).toBeVisible();
  await expect(page.getByText('Choose a validated baseline', {exact: true})).toBeVisible();
  await expect(page.getByText('Cross-cluster DR lane', {exact: true})).toBeVisible();
});

test('reference section exposes grouped lookup navigation', async ({page}) => {
  await page.goto('docs/next/reference');

  await expect(page.getByRole('heading', {name: 'Reference documentation'})).toBeVisible();
  await expect(page.getByText('Quick Checks', {exact: true})).toBeVisible();
  await expect(page.getByText('API Surface', {exact: true})).toBeVisible();
  await expect(page.getByText('Lifecycle & Support Contract', {exact: true})).toBeVisible();
  await expect(page.getByText('Constraints & Caveats', {exact: true})).toBeVisible();
});

test('contribute section exposes grouped contributor navigation', async ({page}) => {
  await page.goto('contribute');
  const sidebar = page.locator('.theme-doc-sidebar-container');

  await expect(page.getByRole('heading', {name: 'Contributor documentation'})).toBeVisible();
  await expect(sidebar.getByRole('link', {name: 'Start Here', exact: true})).toBeVisible();
  await expect(sidebar.getByRole('link', {name: 'Build & Change', exact: true})).toBeVisible();
  await expect(sidebar.getByRole('link', {name: 'Validate & Ship', exact: true})).toBeVisible();
  await expect(sidebar.getByRole('link', {name: 'Project Governance', exact: true})).toBeVisible();
});

test('mobile navigation stays usable', async ({page}) => {
  await page.setViewportSize({width: 390, height: 844});
  await page.goto('');

  await page.getByRole('button', {name: 'Toggle navigation bar'}).click();
  const mobileSidebar = page.locator('.navbar-sidebar--show');
  await expect(mobileSidebar).toBeVisible();
  await expect(mobileSidebar.getByRole('button', {name: 'Docs'}).last()).toBeVisible();
  const contributeLink = mobileSidebar.getByRole('link', {name: 'Contribute', exact: true});
  await expect(contributeLink).toBeVisible();
  await contributeLink.click();
  await expect(page).toHaveURL(/\/openbao-operator\/contribute$/);
});
