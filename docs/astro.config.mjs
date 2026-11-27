import { readFileSync } from 'node:fs';
import { dirname, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

import { defineConfig, passthroughImageService } from 'astro/config';
import starlight from '@astrojs/starlight';

import { writeRecordPages } from './scripts/records.mjs';

/**
 * Where the site is served. Read by the link rewrite below and by Astro.
 * DOCS_BASE serves the same site under another path, such as a beta portal
 * that mounts it behind its own sign-in.
 */
const BASE = process.env.DOCS_BASE || '/docs';

const CONTENT_DIR = fileURLToPath(new URL('./src/content/docs', import.meta.url));

// The index page of the decision records is built from those files on every build and never
// committed. A committed index was a table every new record edited, so two
// pull requests that each added one always conflicted. See
// docs/scripts/records.mjs.
writeRecordPages();

/**
 * Rewrite a relative markdown link to the URL the page is published at.
 *
 * This repository has two readers. The site serves what is under
 * `src/content/docs`, at a route with no extension. The whole repository is
 * also an Obsidian vault, and `.obsidian/app.json` sets `useMarkdownLinks` with
 * `newLinkFormat: relative`, so the form that works there is a relative path to
 * the `.md` file. Astro rewrites neither: a probe page carrying six link shapes
 * emitted all six hrefs into the HTML byte for byte, and there is no markdown
 * link rewriting in `@astrojs/markdown-remark` 7.2.2.
 *
 * So the two forms were mutually exclusive until this ran. With it, the vault
 * form is the one form to author: it resolves in Obsidian as written, and it
 * reaches the right page on the site.
 *
 * A target outside the content directory is left alone. `docs/decisions/` is a
 * real folder the vault opens and the site does not publish, and no link
 * form changes that.
 *
 * See docs/scripts/check-links.mjs, which refuses a link that resolves for
 * neither reader.
 */
function rehypeRelativeMarkdownLinks() {
  const routes = new Map();

  // The route Starlight publishes a page at: its path under the content
  // directory without the extension, with `index` naming its parent, unless the
  // page overrides it with a top-level `slug`. Only depth zero counts, because
  // `sidebar:` has nested keys and none of them is the route.
  const routeOf = (file) => {
    if (routes.has(file)) return routes.get(file);

    let route = null;

    try {
      const body = readFileSync(file, 'utf8');
      const frontmatter = /^---\r?\n([\s\S]*?)\r?\n---/.exec(body);
      const override = frontmatter && /^slug:\s*(.+?)\s*$/m.exec(frontmatter[1]);

      route = override
        ? override[1].replace(/^["']|["']$/g, '').replace(/^\/+|\/+$/g, '')
        : relative(CONTENT_DIR, file)
            .split(/[\\/]/)
            .join('/')
            .replace(/\.mdx?$/, '')
            .replace(/(^|\/)index$/, '');
    } catch {
      route = null;
    }

    routes.set(file, route);

    return route;
  };

  return (tree, vfile) => {
    const source = vfile && vfile.path;

    if (!source) return;

    const walk = (node) => {
      if (node.type === 'element' && node.tagName === 'a' && node.properties) {
        const href = node.properties.href;

        if (typeof href === 'string' && !/^[a-z][a-z0-9+.-]*:/i.test(href) && !href.startsWith('/') && !href.startsWith('#') && !href.startsWith('//')) {
          const at = href.indexOf('#');
          const path = at < 0 ? href : href.slice(0, at);
          const fragment = at < 0 ? '' : href.slice(at);

          if (/\.mdx?$/.test(path)) {
            const target = resolve(dirname(source), decodeURIComponent(path));

            if (!relative(CONTENT_DIR, target).startsWith('..')) {
              const route = routeOf(target);

              if (route !== null) {
                node.properties.href = route === '' ? `${BASE}/${fragment}` : `${BASE}/${route}/${fragment}`;
              }
            }
          }
        }
      }

      for (const child of node.children || []) walk(child);
    };

    walk(tree);
  };
}

/**
 * Give a heading the anchor its author asked for, written as a trailing
 * `{#id}`.
 *
 * A generated reference page is reached from an identifier a reader already
 * holds, so the anchor has to be that identifier rather than a slug of the
 * heading's prose. Astro has no syntax for this: `rehypeHeadingIds` slugs the
 * heading text and only keeps an id that some earlier plugin set, and raw HTML
 * headings are still unparsed when headings are collected, so they never reach
 * the table of contents. A rehype plugin runs before both and gets both.
 *
 * See ADR-20260826-swm.
 */
function rehypeExplicitHeadingIds() {
  const idPattern = /\s*\{#([A-Za-z0-9._:-]+)\}\s*$/;

  return (tree) => {
    const walk = (node) => {
      if (node.type === 'element' && /^h[1-6]$/.test(node.tagName)) {
        const last = node.children[node.children.length - 1];

        if (last && last.type === 'text') {
          const found = idPattern.exec(last.value);

          if (found) {
            last.value = last.value.slice(0, found.index);
            node.properties = node.properties || {};
            node.properties.id = found[1];
          }
        }
      }

      for (const child of node.children || []) walk(child);
    };

    walk(tree);
  };
}

/**
 * Wrap every table in a labelled region that scrolls, and leave the table a
 * table.
 *
 * A table wider than the phone it is read on has to scroll somewhere. Starlight
 * scrolls the table itself, with `display: block; overflow: auto`, which costs
 * two things: a table whose box is no longer a table box, which is what Adrian
 * Roselli measured screen readers dropping table semantics for, and a scroll
 * container with no `tabindex`, which no keyboard can reach.
 *
 * The wrapper is what scrolls instead. `role="region"` with a name puts it in
 * the landmark list, and `tabindex="0"` makes it focusable, which is the pair
 * a scrollable region needs to satisfy SC 2.1.1 and SC 1.3.1. The name is the
 * heading the table sits under, so a screen reader announces which table was
 * reached rather than "region".
 *
 * See ADR-20260826-swm rule 10 and docs/src/styles/tables.css.
 */
function rehypeTableScrollRegion() {
  const textOf = (node) => {
    if (node.type === 'text') return node.value;

    return (node.children || []).map(textOf).join('');
  };

  return (tree) => {
    let heading = '';

    const walk = (node) => {
      const children = node.children || [];

      for (let index = 0; index < children.length; index++) {
        const child = children[index];

        if (child.type === 'element' && /^h[1-6]$/.test(child.tagName)) {
          heading = textOf(child).trim();
        }

        if (child.type === 'element' && child.tagName === 'table') {
          children[index] = {
            type: 'element',
            tagName: 'div',
            properties: {
              className: ['table-scroll'],
              role: 'region',
              'aria-label': heading || 'Table',
              tabIndex: 0,
            },
            children: [child],
          };

          continue;
        }

        walk(child);
      }
    };

    walk(tree);
  };
}

/**
 * Turn a mermaid code block into the figure the client renderer draws into.
 *
 * Starlight hands every code block to Expressive Code, which highlights the
 * mermaid source and leaves it a code block. This plugin runs before that and
 * takes the mermaid ones out of its way: a `figure.mermaid-figure` holding the
 * source in a `pre`. src/scripts/render-diagrams.js finds the figure, draws
 * the diagram and hides the `pre`. A reader with no JavaScript keeps the
 * source.
 *
 * The `pre` holds its text with no `code` element inside it, because
 * Expressive Code claims every `pre` whose one child is a `code`, and it would
 * claim this one back.
 *
 * See docs/scripts/check-mermaid.mjs, which refuses a build where a diagram
 * reached a page without the figure.
 */
function rehypeMermaidFigure() {
  const isMermaid = (node) =>
    node.type === 'element' &&
    node.tagName === 'code' &&
    [].concat(node.properties?.className || []).includes('language-mermaid');

  return (tree) => {
    const walk = (node) => {
      const children = node.children || [];

      for (let index = 0; index < children.length; index++) {
        const child = children[index];

        if (child.type === 'element' && child.tagName === 'pre' && child.children.some(isMermaid)) {
          const code = child.children.find(isMermaid);

          children[index] = {
            type: 'element',
            tagName: 'figure',
            properties: { className: ['mermaid-figure'] },
            children: [
              {
                type: 'element',
                tagName: 'pre',
                properties: { className: ['mermaid-source'] },
                children: code.children,
              },
            ],
          };

          continue;
        }

        walk(child);
      }
    };

    walk(tree);
  };
}

export default defineConfig({
  markdown: {
    rehypePlugins: [
      rehypeExplicitHeadingIds,
      rehypeMermaidFigure,
      rehypeTableScrollRegion,
      rehypeRelativeMarkdownLinks,
    ],
  },
  vite: {
    resolve: {
      dedupe: ['zod'],
    },
  },
  // The screenshots under src/assets are served as they are. The default
  // service resizes them through sharp, a native module this site does not
  // install, and a build without it stops on the first image.
  image: { service: passthroughImageService() },
  integrations: [
    starlight({
      title: 'Prusa Connect',
      description:
        'Local server for Prusa printers: WebSocket backend, HTTP REST API, and real-time events.',
      social: [],
      customCss: ['./src/styles/tables.css', './src/styles/mermaid.css'],
      // The override adds the loader for the diagrams and renders Starlight's
      // own head after it.
      components: { Head: './src/components/Head.astro' },
      sidebar: [
        {
          label: 'Overview',
          items: [
            { label: 'Introduction', slug: 'index' },
            { label: 'Install from a release', slug: 'install-from-a-release' },
            { label: 'Run the server from source', slug: 'getting-started' },
            { label: 'Walk through the demo', slug: 'demo-walkthrough' },
            { label: 'Write your first module', slug: 'writing-your-first-module' },
            { label: 'Use the API from a page', slug: 'using-the-api-from-a-page' },
          ],
        },
        {
          // ADR-20261015-ejz rule 41. Every part of the server is configurable, so
          // the configuration comes right after the overview, one page per
          // part, and the setup wizard links each key to its row there.
          label: 'Configuration',
          items: [
            { label: 'How the configuration works', slug: 'config' },
            { label: 'Network and API', slug: 'config/network' },
            { label: 'Encryption', slug: 'config/encryption' },
            { label: 'Sign-in and access', slug: 'config/sign-in' },
            { label: 'Web app', slug: 'config/web-app' },
            { label: 'Discovery', slug: 'config/discovery' },
            { label: 'Printers', slug: 'config/printers' },
            { label: 'Storage, history and integrations', slug: 'config/resources' },
            { label: 'Database', slug: 'config/database' },
            { label: 'Logging and metrics', slug: 'config/logging' },
            { label: 'Extensions', slug: 'config/extensions' },
            { label: 'Server internals', slug: 'config/server' },
          ],
        },
        {
          label: 'Guides',
          items: [
            { label: 'Upgrade, back up and uninstall', slug: 'upgrade-back-up-and-uninstall' },
            { label: 'Container deployment', slug: 'container' },
            { label: 'Run it on a NAS', slug: 'nas' },
            { label: 'GraphQL', slug: 'graphql' },
            { label: 'Human authentication', slug: 'human-auth' },
            { label: 'Local accounts', slug: 'local-accounts' },
            { label: 'Scoped credentials and app pairing', slug: 'scoped-credentials-and-app-pairing' },
            { label: 'Home Assistant', slug: 'home-assistant' },
            { label: 'Dashboards', slug: 'dashboards' },
            { label: 'Storage', slug: 'storage' },
            { label: 'Adapters, modules and runtimes', slug: 'extension-concepts' },
            { label: 'Writing a printer driver', slug: 'writing-a-driver' },
            { label: 'Writing a service module', slug: 'writing-a-module' },
            { label: 'Styling a module', slug: 'styling-a-module' },
            { label: 'Prusa xBuddy websocket protocol', slug: 'prusa-xbuddy-websocket-protocol' },
            { label: 'Logging', slug: 'logging' },
            { label: 'Metrics', slug: 'metrics' },
            { label: 'Tracing', slug: 'tracing' },
            { label: 'Printer configuration', slug: 'printer-configuration' },
            { label: 'Pair a printer at its screen', slug: 'pair-a-printer-at-its-screen' },
            { label: 'Bench preflight', slug: 'bench-preflight' },
            { label: 'Certificates', slug: 'certificates' },
            { label: 'Printer simulator', slug: 'emulator' },
            { label: 'Kubernetes', slug: 'kubernetes' },
          ],
        },
        {
          label: 'Design decisions',
          collapsed: true,
          items: [
            { label: 'Overview', slug: 'design' },
            { label: 'The core knows devices', slug: 'design/the-core-knows-devices' },
            { label: 'Features are modules', slug: 'design/features-are-modules' },
            { label: 'The SDK is the contract', slug: 'design/the-sdk-is-the-contract' },
            { label: 'A driver declares', slug: 'design/a-driver-declares' },
            { label: 'One registry', slug: 'design/one-registry' },
            { label: 'Permissions', slug: 'design/permissions' },
            { label: 'Events', slug: 'design/events' },
            { label: 'No history in the core', slug: 'design/no-history-in-the-core' },
            { label: 'API compatibility', slug: 'design/api-compatibility' },
            { label: 'Modules ask', slug: 'design/modules-ask' },
          ],
        },
        {
          // One page per folder under extensions/, read from the directory.
          // make generate-sdk-reference writes the facts on each page from the
          // extension's module.yaml and openapi.json.
          label: 'Official extensions',
          items: [{ autogenerate: { directory: 'reference/extensions' } }],
        },
        {
          // ADR-20260910-he3 rule 13. The two reference groups are read from their
          // directories: one page per OpenAPI tag, in the order of the
          // `sidebar.order` the generator writes from the document's tag
          // order, and one page per sdk package in name order. A page
          // `make generate-sdk-reference` creates appears here with no edit.
          label: 'API reference',
          items: [{ autogenerate: { directory: 'reference/api' } }],
        },
        {
          label: 'Go SDK',
          items: [{ autogenerate: { directory: 'reference/go-sdk' } }],
        },
        {
          label: 'Reference data',
          items: [
            { label: 'Error codes', slug: 'error-codes' },
            { label: 'Module slots', slug: 'reference/module-slots' },
            { label: 'Event types', slug: 'event-types' },
            { label: 'Authorization vocabulary', slug: 'authorization-vocabulary' },
            { label: 'Capability vocabulary', slug: 'capabilities' },
            { label: 'Resource kinds', slug: 'resource-kinds' },
            { label: 'Extensions', slug: 'extensions' },
            { label: 'Glossary', slug: 'glossary' },
            { label: 'SSE events', slug: 'sse' },
            { label: 'SSE event reference', slug: 'sse-asyncapi' },
            { label: 'MQTT', slug: 'mqtt' },
            { label: 'MQTT topic reference', slug: 'mqtt-asyncapi' },
            { label: 'The connector contract', slug: 'connector-contract' },
            { label: 'How extensions work', slug: 'plugin-architecture' },
            { label: 'The plugin wire protocol', slug: 'plugin-wire-protocol' },
          ],
        },
        {
          label: 'Onboarding',
          items: [
            { label: 'The shape of the system', slug: 'onboarding' },
            { label: 'The entry point', slug: 'onboarding/the-entry-point' },
            { label: 'The model', slug: 'onboarding/the-model' },
            { label: 'The data flow', slug: 'onboarding/the-data-flow' },
            { label: 'The SDK', slug: 'onboarding/the-sdk' },
            { label: 'Registries and names', slug: 'onboarding/registries-and-names' },
            { label: 'Where things live', slug: 'onboarding/where-things-live' },
            { label: 'When things go wrong', slug: 'onboarding/when-things-go-wrong' },
          ],
        },
        {
          // Built by docs/scripts/records.mjs on every build and never
          // committed.
          label: 'Records',
          items: [
            { label: 'Decisions', slug: 'records/decisions' },
          ],
        },
        {
          label: 'Internals',
          items: [
            { label: 'The application core', slug: 'internals/the-application-core' },
            { label: 'The authorization graph', slug: 'internals/authorization' },
            { label: 'Transport and the stream', slug: 'internals/transport-and-the-stream' },
            { label: 'Printers and drivers', slug: 'internals/printers-and-drivers' },
            { label: 'Build, gates and testing', slug: 'internals/build-gates-and-testing' },
            { label: 'The sdk and extensions', slug: 'internals/the-sdk-and-extensions' },
            { label: 'The extension host', slug: 'internals/the-extension-host' },
            { label: 'The Fractal link', slug: 'internals/the-fractal-link' },
          ],
        },
        {
          // Four pages describing the retired `serve` server or a design that
          // was never built. Each says so at the top. They sit in their own
          // group so a reader does not meet one while looking for the core.
          // ADR-20260826-53y: a reader-facing tier does not hold a design nothing runs.
          label: 'The retired server',
          collapsed: true,
          items: [
            { label: 'Architecture', slug: 'architecture' },
            { label: 'Creating connectors', slug: 'creating-connectors' },
            { label: 'The firmware support matrix', slug: 'prusa-command-architecture' },
          ],
        },
      ],
    }),
  ],
  base: BASE,
  outDir: './dist',
});
