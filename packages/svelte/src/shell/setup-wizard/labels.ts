// The names a person reads for each configuration key and each section. The
// sentence under a field is the server's own, from the configuration
// reference; the name above it is ours, and a key this page does not know yet
// shows its dotted path.
import { t } from '../../i18n';
import type { SetupKey } from '../../data';
import type { SectionId } from './model';

const KEY_LABEL: Readonly<Record<string, string>> = {
  'core.api.addr': 'setupWizard.key.apiAddr',
  'core.frontend.enabled': 'setupWizard.key.webApp',
  'core.api.tls.certificate': 'setupWizard.key.tlsCertificate',
  'core.api.tls.key': 'setupWizard.key.tlsKey',
  'core.api.allowed_origins': 'setupWizard.key.allowedOrigins',
  'core.api.request_timeout': 'setupWizard.key.requestTimeout',
  'core.api.stream_timeout': 'setupWizard.key.streamTimeout',
  'core.log.level': 'setupWizard.key.logLevel',
  'core.log.format': 'setupWizard.key.logFormat',
  'core.frontend.title': 'setupWizard.key.title',
  'core.frontend.default_language': 'setupWizard.key.language',
  'core.frontend.theme': 'setupWizard.key.theme',
  'core.frontend.printer_address': 'setupWizard.key.printerAddress',
  'core.frontend.printer_custom_ca': 'setupWizard.key.printerCustomCa',
  'core.discovery.enabled': 'setupWizard.key.discovery',
  'core.discovery.services': 'setupWizard.key.discoveryServices',
  'core.discovery.hidden': 'setupWizard.key.discoveryHidden',
  'core.dashboards.enabled': 'setupWizard.key.dashboards',
  'core.observability.enabled': 'setupWizard.key.metrics',
  'core.observability.addr': 'setupWizard.key.metricsAddr',
  'core.marketplace.enabled': 'setupWizard.key.marketplace',
  'core.marketplace.url': 'setupWizard.key.marketplaceUrl',
  'core.marketplace.timeout': 'setupWizard.key.marketplaceTimeout',
  'core.principal': 'setupWizard.key.principal',
  'core.auth.issuer': 'setupWizard.key.issuer',
  'core.auth.client_id': 'setupWizard.key.clientId',
  'core.auth.redirect_uri': 'setupWizard.key.redirectUri',
  'core.auth.local': 'setupWizard.key.localLogin',
  'core.auth.password_memory_bytes': 'setupWizard.key.passwordMemory',
  'core.auth.password_passes': 'setupWizard.key.passwordPasses',
  'core.auth.password_hashes_at_once': 'setupWizard.key.passwordHashes',
  'core.authz.org_admin_handover': 'setupWizard.key.adminHandover',
  'core.trusted_proxies': 'setupWizard.key.trustedProxies',
  'core.site': 'setupWizard.key.site',
  'core.events.heartbeat': 'setupWizard.key.heartbeat',
  'core.events.retry': 'setupWizard.key.retry',
  'core.events.max_streams_per_principal': 'setupWizard.key.maxStreams',
  'core.device_state_budget': 'setupWizard.key.stateBudget',
  'core.module_settings_bytes': 'setupWizard.key.moduleSettings',
  'core.preview_cache_bytes': 'setupWizard.key.previewCache',
  'core.device_listing_cache_bytes': 'setupWizard.key.listingCache',
  'core.device_thumbnail_disk_bytes': 'setupWizard.key.thumbnailDiskBytes',
  'core.device_thumbnail_disk': 'setupWizard.key.thumbnailDisk',
  'core.device_thumbnail_prefetch': 'setupWizard.key.thumbnailPrefetch',
  'core.memory_limit': 'setupWizard.key.memoryLimit',
  'core.shutdown_timeout': 'setupWizard.key.shutdownTimeout',
  'persistence.type': 'setupWizard.key.engine',
  'persistence.max_open_conns': 'setupWizard.key.maxOpen',
  'persistence.max_idle_conns': 'setupWizard.key.maxIdle',
  'persistence.conn_max_lifetime': 'setupWizard.key.connLifetime',
  'persistence.sqlite.page_cache_bytes': 'setupWizard.key.pageCache',
  'persistence.postgres.dsn': 'setupWizard.key.dsn',
  'persistence.postgres.statement_timeout': 'setupWizard.key.statementTimeout',
};

// Every key gets a sentence of ours, written for somebody who has never read
// the reference. The reference's own sentence waits behind the info button.
const KEY_HINT: Readonly<Record<string, string>> = {
  'core.api.addr': 'setupWizard.hint.apiAddr',
  'core.frontend.enabled': 'setupWizard.hint.webApp',
  'core.api.tls.certificate': 'setupWizard.hint.tlsCertificate',
  'core.api.tls.key': 'setupWizard.hint.tlsKey',
  'core.api.allowed_origins': 'setupWizard.hint.allowedOrigins',
  'core.api.request_timeout': 'setupWizard.hint.requestTimeout',
  'core.api.stream_timeout': 'setupWizard.hint.streamTimeout',
  'core.log.level': 'setupWizard.hint.logLevel',
  'core.log.format': 'setupWizard.hint.logFormat',
  'core.frontend.title': 'setupWizard.hint.serverName',
  'core.frontend.default_language': 'setupWizard.hint.language',
  'core.frontend.theme': 'setupWizard.hint.theme',
  'core.frontend.printer_address': 'setupWizard.hint.printerAddress',
  'core.frontend.printer_custom_ca': 'setupWizard.hint.printerCustomCa',
  'core.discovery.enabled': 'setupWizard.hint.discovery',
  'core.discovery.services': 'setupWizard.hint.discoveryServices',
  'core.discovery.hidden': 'setupWizard.hint.discoveryHidden',
  'core.dashboards.enabled': 'setupWizard.hint.dashboards',
  'core.observability.enabled': 'setupWizard.hint.metrics',
  'core.observability.addr': 'setupWizard.hint.metricsAddr',
  'core.marketplace.enabled': 'setupWizard.hint.marketplace',
  'core.marketplace.url': 'setupWizard.hint.marketplaceUrl',
  'core.marketplace.timeout': 'setupWizard.hint.marketplaceTimeout',
  'core.principal': 'setupWizard.hint.principal',
  'core.auth.issuer': 'setupWizard.hint.issuer',
  'core.auth.client_id': 'setupWizard.hint.clientId',
  'core.auth.redirect_uri': 'setupWizard.hint.redirectUri',
  'core.auth.local': 'setupWizard.hint.localLogin',
  'core.auth.password_memory_bytes': 'setupWizard.hint.passwordMemory',
  'core.auth.password_passes': 'setupWizard.hint.passwordPasses',
  'core.auth.password_hashes_at_once': 'setupWizard.hint.passwordHashes',
  'core.authz.org_admin_handover': 'setupWizard.hint.adminHandover',
  'core.trusted_proxies': 'setupWizard.hint.trustedProxies',
  'core.site': 'setupWizard.hint.site',
  'core.events.heartbeat': 'setupWizard.hint.heartbeat',
  'core.events.retry': 'setupWizard.hint.retry',
  'core.events.max_streams_per_principal': 'setupWizard.hint.maxStreams',
  'core.device_state_budget': 'setupWizard.hint.stateBudget',
  'core.module_settings_bytes': 'setupWizard.hint.moduleSettings',
  'core.preview_cache_bytes': 'setupWizard.hint.previewCache',
  'core.device_listing_cache_bytes': 'setupWizard.hint.listingCache',
  'core.device_thumbnail_disk_bytes': 'setupWizard.hint.thumbnailDiskBytes',
  'core.device_thumbnail_disk': 'setupWizard.hint.thumbnailDisk',
  'core.device_thumbnail_prefetch': 'setupWizard.hint.thumbnailPrefetch',
  'core.memory_limit': 'setupWizard.hint.memoryLimit',
  'core.shutdown_timeout': 'setupWizard.hint.shutdownTimeout',
  'persistence.type': 'setupWizard.hint.engine',
  'persistence.max_open_conns': 'setupWizard.hint.maxOpen',
  'persistence.max_idle_conns': 'setupWizard.hint.maxIdle',
  'persistence.conn_max_lifetime': 'setupWizard.hint.connLifetime',
  'persistence.sqlite.page_cache_bytes': 'setupWizard.hint.pageCache',
  'persistence.postgres.dsn': 'setupWizard.hint.dsn',
  'persistence.postgres.statement_timeout': 'setupWizard.hint.statementTimeout',
};

const SECTION_TITLE: Readonly<Record<SectionId, string>> = {
  network: 'setupWizard.section.network.title',
  look: 'setupWizard.section.look.title',
  access: 'setupWizard.section.access.title',
  database: 'setupWizard.section.database.title',
  logging: 'setupWizard.section.logging.title',
  server: 'setupWizard.section.server.title',
};

const SECTION_LEAD: Readonly<Record<SectionId, string>> = {
  network: 'setupWizard.section.network.lead',
  look: 'setupWizard.section.look.lead',
  access: 'setupWizard.section.access.lead',
  database: 'setupWizard.section.database.lead',
  logging: 'setupWizard.section.logging.lead',
  server: 'setupWizard.section.server.lead',
};

const SECTION_SHORT: Readonly<Record<SectionId, string>> = {
  network: 'setupWizard.section.network.short',
  look: 'setupWizard.section.look.short',
  access: 'setupWizard.section.access.short',
  database: 'setupWizard.section.database.short',
  logging: 'setupWizard.section.logging.short',
  server: 'setupWizard.section.server.short',
};

/** The name above a field. */
export const labelOf = (key: SetupKey): string => {
  const label = KEY_LABEL[key.path];

  return label === undefined ? key.path : t(label);
};

/** The sentence under a field. */
export const hintOf = (key: SetupKey): string => {
  const hint = KEY_HINT[key.path];

  return hint === undefined ? key.doc : t(hint);
};

/** A section's step title, which is the question it asks. */
export const sectionTitle = (section: SectionId): string =>
  t(SECTION_TITLE[section]);

/** The sentence under a section's title. */
export const sectionLead = (section: SectionId): string =>
  t(SECTION_LEAD[section]);

/** A section's name in the step list. */
export const sectionShort = (section: SectionId): string =>
  t(SECTION_SHORT[section]);
