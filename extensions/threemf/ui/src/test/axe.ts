import axe from 'axe-core';

/**
 * Run axe over a rendered element and fail with what it found.
 *
 * A component test renders one subtree and not a document, so axe skips the
 * rules that judge a whole page: a single main landmark, a page title, a
 * language on the root element. What is left is what a component decides for
 * itself, which is the part this helper holds.
 */
export const expectNoViolations = async (element: Element): Promise<void> => {
  // Two rules are off, each for its own reason.
  //
  // `region` asks that every piece of content sit inside a landmark. A
  // landmark is the page's job and a component never renders one, so the rule
  // fires on every component and says nothing about the component.
  //
  // `color-contrast` measures what the browser painted, which under a full
  // suite can be a surface still animating in. `styles/contrast.test.ts`
  // already holds every text pair to WCAG AA at 4.5:1 and every non-text mark
  // to 1.4.11 at 3:1, in both themes, from the declared values
  // rather than from a paint. So this rule adds no coverage and reports a
  // different answer on a loaded machine.
  const { violations } = await axe.run(element, {
    rules: { region: { enabled: false }, 'color-contrast': { enabled: false } },
  });
  if (violations.length === 0) return;

  // The rule id, the impact and the selector of each node. A message that
  // reports a count alone sends the reader back to the browser to find out
  // which element failed.
  const report = violations
    .map((violation) => {
      const nodes = violation.nodes
        .map((node) => `      ${node.target.join(' ')}`)
        .join('\n');
      return `  ${violation.id} (${violation.impact ?? 'unknown'}): ${violation.help}\n${nodes}`;
    })
    .join('\n');

  throw new Error(
    `axe found ${String(violations.length)} violation(s):\n${report}`,
  );
};
