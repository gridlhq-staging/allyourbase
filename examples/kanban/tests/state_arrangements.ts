import { type Locator, type Page, expect } from "@playwright/test";

export async function dragCardToColumn(
  page: Page,
  card: Locator,
  destination: Locator,
): Promise<void> {
  const cardBox = await card.boundingBox();
  const destinationBox = await destination.boundingBox();
  expect(cardBox).toBeTruthy();
  expect(destinationBox).toBeTruthy();

  await page.mouse.move(
    cardBox!.x + cardBox!.width / 2,
    cardBox!.y + cardBox!.height / 2,
  );
  await page.mouse.down();
  await page.mouse.move(
    destinationBox!.x + destinationBox!.width / 2,
    destinationBox!.y + destinationBox!.height / 2,
    { steps: 10 },
  );
  await page.mouse.up();
}
