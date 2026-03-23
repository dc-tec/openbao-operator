import {expect, test, type Page} from '@playwright/test';

async function forceTheme(page: Page, theme: 'light' | 'dark') {
  await page.addInitScript((value) => {
    window.localStorage.setItem('theme', value);
  }, theme);
}

async function contrastRatio(page: Page, selector: string) {
  return page.locator(selector).first().evaluate((element) => {
    const parseRgb = (value: string) => {
      const matches = value.match(/\d+(\.\d+)?/g) ?? ['0', '0', '0'];
      return matches.slice(0, 4).map(Number);
    };

    const luminance = ([r, g, b]: number[]) => {
      const [rs, gs, bs] = [r, g, b].map((channel) => {
        const normalized = channel / 255;
        return normalized <= 0.03928
          ? normalized / 12.92
          : ((normalized + 0.055) / 1.055) ** 2.4;
      });
      return 0.2126 * rs + 0.7152 * gs + 0.0722 * bs;
    };

    const resolveBackground = (node: Element | null): number[] => {
      let current: Element | null = node;
      while (current) {
        const style = window.getComputedStyle(current);
        const rgba = parseRgb(style.backgroundColor);
        const alpha = rgba[3] ?? 1;
        if (alpha > 0) {
          return rgba;
        }
        current = current.parentElement;
      }
      return [255, 255, 255, 1];
    };

    const style = window.getComputedStyle(element);
    const foreground = luminance(parseRgb(style.color));
    const background = luminance(resolveBackground(element));
    const lighter = Math.max(foreground, background);
    const darker = Math.min(foreground, background);
    return (lighter + 0.05) / (darker + 0.05);
  });
}

test('homepage light theme uses the light surface palette', async ({page}) => {
  await forceTheme(page, 'light');
  await page.goto('');

  await expect(page.locator('html')).toHaveAttribute('data-theme', 'light');

  const bodyBackground = await page.locator('body').evaluate((element) => {
    return window.getComputedStyle(element).backgroundColor;
  });
  expect(bodyBackground).toBe('rgb(242, 239, 232)');

  const heroTitleColor = await page.locator('main h1').first().evaluate((element) => {
    return window.getComputedStyle(element).color;
  });
  expect(heroTitleColor).toBe('rgb(16, 34, 48)');
});

test('light theme keeps decision-table and command-block surfaces readable', async ({page}) => {
  await forceTheme(page, 'light');
  await page.goto('docs/next/configure/security-profiles');

  const headerContrast = await contrastRatio(page, '.decisionTable thead th:first-child');
  expect(headerContrast).toBeGreaterThan(4.5);

  const commandTitleContrast = await contrastRatio(page, '.commandBlock__title');
  expect(commandTitleContrast).toBeGreaterThan(4.5);

  const commandMetaContrast = await contrastRatio(page, '.commandBlock__meta');
  expect(commandMetaContrast).toBeGreaterThan(4.5);

  const codeSurface = await page
    .locator('.commandBlock__code .theme-code-block')
    .first()
    .evaluate((element) => {
      return window.getComputedStyle(element).backgroundColor;
    });
  expect(codeSurface).not.toBe('rgba(0, 0, 0, 0)');
});

test('mobile journey hero stacks instead of squeezing into two columns', async ({page}) => {
  await page.setViewportSize({width: 390, height: 844});
  await page.goto('docs/next/get-started');

  const hero = page.locator('.pageHero').first();
  const heroColumns = await hero.evaluate((element) => window.getComputedStyle(element).gridTemplateColumns);
  expect(heroColumns.trim().split(/\s+/)).toHaveLength(1);

  const contentBox = await page.locator('.pageHero__content').boundingBox();
  const asideBox = await page.locator('.pageHero__aside').boundingBox();
  const headingBox = await page.getByRole('heading', {level: 1}).boundingBox();

  expect(contentBox).not.toBeNull();
  expect(asideBox).not.toBeNull();
  expect(headingBox).not.toBeNull();

  expect(Math.abs((contentBox?.x ?? 0) - (asideBox?.x ?? 0))).toBeLessThan(8);
  expect((asideBox?.y ?? 0)).toBeGreaterThan((contentBox?.y ?? 0) + (contentBox?.height ?? 0) - 8);
  expect(headingBox?.width ?? 0).toBeGreaterThan(250);
});
