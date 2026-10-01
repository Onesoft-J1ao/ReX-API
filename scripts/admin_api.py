#!/usr/bin/env python3
"""ReX-API admin client.

Every admin/root endpoint of the instance behind one token:

    export REX_ADMIN_TOKEN=<your admin or root token>
    python admin_api.py list                     # all 230 endpoints
    python admin_api.py channels                 # channel list
    python admin_api.py call GET /api/option/

The endpoint table is generated from the router itself, so ``list`` is the
authoritative surface rather than a hand-maintained subset. Anything not
covered by a shortcut is still reachable through ``call``.

Conventions:
  * output is JSON on stdout (an agent can parse it directly); ``-o table``
    renders a human table where one makes sense
  * a non-2xx response exits non-zero and prints the server message on stderr
  * ``Authorization`` carries the token; no session cookie is needed
"""

from __future__ import annotations

import argparse
import json
import os
import re
import ssl
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

DEFAULT_BASE_URL = "https://ai.j1ao.vip"
TOKEN_ENV = "REX_ADMIN_TOKEN"
BASE_URL_ENV = "REX_BASE_URL"
UA_ENV = "REX_USER_AGENT"

# The instance sits behind a WAF that bans the stock urllib signature
# (Cloudflare 1010), so a browser-like agent is the default and can be
# overridden with --user-agent for a deployment that wants a different one.
DEFAULT_USER_AGENT = (
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "
    "(KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
)

# (method, path, required auth, handler) — generated from router registration.
ROUTES: tuple[tuple[str, str, str, str], ...] = (
    ('GET', '/api/audit', 'admin', 'GetAuditLogs'),
    ('GET', '/api/audit/export', 'admin', 'ExportAuditLogsCSV'),
    ('GET', '/api/authz/catalog', 'admin', 'GetPermissionCatalog'),
    ('GET', '/api/channel/', 'admin', 'GetAllChannels'),
    ('POST', '/api/channel/', 'admin', 'AddChannel'),
    ('PUT', '/api/channel/', 'admin', 'UpdateChannel'),
    ('DELETE', '/api/channel/:id', 'admin', 'DeleteChannel'),
    ('GET', '/api/channel/:id', 'admin', 'GetChannel'),
    ('POST', '/api/channel/:id/codex/refresh', 'admin', 'RefreshCodexChannelCredential'),
    ('GET', '/api/channel/:id/codex/usage', 'admin', 'GetCodexChannelUsage'),
    ('POST', '/api/channel/:id/codex/usage/reset', 'admin', 'ResetCodexChannelUsage'),
    ('GET', '/api/channel/:id/codex/usage/reset-credits', 'admin', 'GetCodexChannelRateLimitResetCredits'),
    ('POST', '/api/channel/:id/key', 'root', 'GetChannelKey'),
    ('GET', '/api/channel/:id/sglang/status', 'admin', 'GetSGLangChannelStatus'),
    ('POST', '/api/channel/:id/status', 'admin', 'UpdateChannelStatus'),
    ('GET', '/api/channel/:id/vllm/status', 'admin', 'GetVLLMChannelStatus'),
    ('POST', '/api/channel/batch', 'admin', 'DeleteChannelBatch'),
    ('POST', '/api/channel/batch/tag', 'admin', 'BatchSetChannelTag'),
    ('POST', '/api/channel/copy/:id', 'admin', 'CopyChannel'),
    ('GET', '/api/channel/default_base_urls', 'admin', 'GetChannelDefaultBaseURLs'),
    ('DELETE', '/api/channel/disabled', 'admin', 'DeleteDisabledChannel'),
    ('POST', '/api/channel/fetch_models', 'admin', 'FetchModels'),
    ('GET', '/api/channel/fetch_models/:id', 'admin', 'FetchUpstreamModels'),
    ('POST', '/api/channel/fix', 'admin', 'FixChannelsAbilities'),
    ('GET', '/api/channel/models', 'admin', 'ChannelListModels'),
    ('GET', '/api/channel/models_enabled', 'admin', 'EnabledListModels'),
    ('POST', '/api/channel/multi_key/manage', 'admin', 'ManageMultiKeys'),
    ('DELETE', '/api/channel/ollama/delete', 'admin', 'OllamaDeleteModel'),
    ('POST', '/api/channel/ollama/pull', 'admin', 'OllamaPullModel'),
    ('POST', '/api/channel/ollama/pull/stream', 'admin', 'OllamaPullModelStream'),
    ('GET', '/api/channel/ollama/version/:id', 'admin', 'OllamaVersion'),
    ('GET', '/api/channel/ops', 'admin', 'GetChannelOps'),
    ('GET', '/api/channel/search', 'admin', 'SearchChannels'),
    ('POST', '/api/channel/status/batch', 'admin', 'BatchUpdateChannelStatus'),
    ('POST', '/api/channel/status_sync/apply', 'admin', 'ApplyChannelStatusSync'),
    ('POST', '/api/channel/status_sync/preview', 'admin', 'PreviewChannelStatusSync'),
    ('PUT', '/api/channel/tag', 'admin', 'EditTagChannels'),
    ('POST', '/api/channel/tag/disabled', 'admin', 'DisableTagChannels'),
    ('POST', '/api/channel/tag/enabled', 'admin', 'EnableTagChannels'),
    ('GET', '/api/channel/tag/models', 'admin', 'GetTagModels'),
    ('GET', '/api/channel/test', 'admin', 'TestAllChannels'),
    ('GET', '/api/channel/test/:id', 'admin', 'TestChannel'),
    ('GET', '/api/channel/update_balance', 'admin', 'UpdateAllChannelsBalance'),
    ('GET', '/api/channel/update_balance/:id', 'admin', 'UpdateChannelBalance'),
    ('POST', '/api/channel/upstream_updates/apply', 'admin', 'ApplyChannelUpstreamModelUpdates'),
    ('POST', '/api/channel/upstream_updates/apply_all', 'admin', 'ApplyAllChannelUpstreamModelUpdates'),
    ('POST', '/api/channel/upstream_updates/detect', 'admin', 'DetectChannelUpstreamModelUpdates'),
    ('POST', '/api/channel/upstream_updates/detect_all', 'admin', 'DetectAllChannelUpstreamModelUpdates'),
    ('GET', '/api/custom-oauth-provider/', 'root', 'GetCustomOAuthProviders'),
    ('POST', '/api/custom-oauth-provider/', 'root', 'CreateCustomOAuthProvider'),
    ('DELETE', '/api/custom-oauth-provider/:id', 'root', 'DeleteCustomOAuthProvider'),
    ('GET', '/api/custom-oauth-provider/:id', 'root', 'GetCustomOAuthProvider'),
    ('PUT', '/api/custom-oauth-provider/:id', 'root', 'UpdateCustomOAuthProvider'),
    ('POST', '/api/custom-oauth-provider/discovery', 'root', 'FetchCustomOAuthDiscovery'),
    ('GET', '/api/data/', 'admin', 'GetAllQuotaDates'),
    ('GET', '/api/data/flow', 'admin', 'GetAllFlowQuotaDates'),
    ('GET', '/api/data/users', 'admin', 'GetQuotaDatesByUser'),
    ('GET', '/api/deployments/', 'admin', 'GetAllDeployments'),
    ('POST', '/api/deployments/', 'admin', 'CreateDeployment'),
    ('DELETE', '/api/deployments/:id', 'admin', 'DeleteDeployment'),
    ('GET', '/api/deployments/:id', 'admin', 'GetDeployment'),
    ('PUT', '/api/deployments/:id', 'admin', 'UpdateDeployment'),
    ('GET', '/api/deployments/:id/containers', 'admin', 'ListDeploymentContainers'),
    ('GET', '/api/deployments/:id/containers/:container_id', 'admin', 'GetContainerDetails'),
    ('POST', '/api/deployments/:id/extend', 'admin', 'ExtendDeployment'),
    ('GET', '/api/deployments/:id/logs', 'admin', 'GetDeploymentLogs'),
    ('PUT', '/api/deployments/:id/name', 'admin', 'UpdateDeploymentName'),
    ('GET', '/api/deployments/available-replicas', 'admin', 'GetAvailableReplicas'),
    ('GET', '/api/deployments/check-name', 'admin', 'CheckClusterNameAvailability'),
    ('GET', '/api/deployments/hardware-types', 'admin', 'GetHardwareTypes'),
    ('GET', '/api/deployments/locations', 'admin', 'GetLocations'),
    ('POST', '/api/deployments/price-estimation', 'admin', 'GetPriceEstimation'),
    ('GET', '/api/deployments/search', 'admin', 'SearchDeployments'),
    ('GET', '/api/deployments/settings', 'admin', 'GetModelDeploymentSettings'),
    ('POST', '/api/deployments/settings/test-connection', 'admin', 'TestIoNetConnection'),
    ('POST', '/api/deployments/test-connection', 'admin', 'TestIoNetConnection'),
    ('GET', '/api/group/', 'admin', 'GetGroups'),
    ('GET', '/api/linked-service/', 'root', 'ListLinkedServices'),
    ('POST', '/api/linked-service/', 'root', 'CreateLinkedService'),
    ('DELETE', '/api/linked-service/:id', 'root', 'DeleteLinkedService'),
    ('GET', '/api/linked-service/:id', 'root', 'GetLinkedService'),
    ('PUT', '/api/linked-service/:id', 'root', 'UpdateLinkedService'),
    ('POST', '/api/linked-service/:id/sync-password', 'root', 'SyncLinkedServicePassword'),
    ('GET', '/api/log/', 'admin', 'GetAllLogs'),
    ('GET', '/api/log/channel_affinity_usage_cache', 'admin', 'GetChannelAffinityUsageCacheStats'),
    ('GET', '/api/log/export', 'admin', 'ExportLogsCSV'),
    ('GET', '/api/log/search', 'admin', 'SearchAllLogs'),
    ('GET', '/api/log/stat', 'admin', 'GetLogsStat'),
    ('GET', '/api/mj/', 'admin', 'GetAllMidjourney'),
    ('GET', '/api/mj/export', 'admin', 'ExportMidjourneyCSV'),
    ('GET', '/api/models', 'admin', 'DashboardListModels'),
    ('GET', '/api/models/', 'admin', 'GetAllModelsMeta'),
    ('POST', '/api/models/', 'admin', 'CreateModelMeta'),
    ('PUT', '/api/models/', 'admin', 'UpdateModelMeta'),
    ('DELETE', '/api/models/:id', 'admin', 'DeleteModelMeta'),
    ('GET', '/api/models/:id', 'admin', 'GetModelMeta'),
    ('POST', '/api/models/delete', 'admin', 'BatchDeleteModelMeta'),
    ('DELETE', '/api/models/delete-unused', 'admin', 'DeleteUnusedModelsMeta'),
    ('GET', '/api/models/missing', 'admin', 'GetMissingModels'),
    ('GET', '/api/models/search', 'admin', 'SearchModelsMeta'),
    ('POST', '/api/models/sync_upstream', 'admin', 'SyncUpstreamModels'),
    ('GET', '/api/models/sync_upstream/preview', 'admin', 'SyncUpstreamPreview'),
    ('GET', '/api/option/', 'root', 'GetOptions'),
    ('PUT', '/api/option/', 'root', 'UpdateOption'),
    ('DELETE', '/api/option/channel_affinity_cache', 'root', 'ClearChannelAffinityCache'),
    ('GET', '/api/option/channel_affinity_cache', 'root', 'GetChannelAffinityCacheStats'),
    ('GET', '/api/option/model_pricing', 'root', 'GetModelPricingConfig'),
    ('PATCH', '/api/option/model_pricing', 'root', 'UpdateModelPricingConfig'),
    ('POST', '/api/option/model_pricing/preview', 'root', 'PreviewModelPricing'),
    ('PUT', '/api/option/passkey/domains', 'root', 'UpdatePasskeyDomains'),
    ('POST', '/api/option/payment_compliance', 'root', 'ConfirmPaymentCompliance'),
    ('GET', '/api/option/request_policy', 'root', 'GetRequestPolicy'),
    ('PATCH', '/api/option/request_policy', 'root', 'UpdateRequestPolicy'),
    ('POST', '/api/option/rest_model_ratio', 'root', 'ResetModelRatio'),
    ('POST', '/api/option/smtp/test', 'root', 'SendSMTPTestEmail'),
    ('DELETE', '/api/option/support', 'root', 'ClearSupportData'),
    ('GET', '/api/option/waffo-pancake/catalog', 'root', 'ListWaffoPancakeCatalog'),
    ('POST', '/api/option/waffo-pancake/pair', 'root', 'CreateWaffoPancakePair'),
    ('POST', '/api/option/waffo-pancake/save', 'root', 'SaveWaffoPancake'),
    ('POST', '/api/option/waffo-pancake/subscription-product', 'root', 'CreateWaffoPancakeSubscriptionProduct'),
    ('GET', '/api/option/waffo-pancake/subscription-product-options', 'root', 'ListWaffoPancakeSubscriptionProductOptions'),
    ('DELETE', '/api/performance/disk_cache', 'root', 'ClearDiskCache'),
    ('POST', '/api/performance/gc', 'root', 'ForceGC'),
    ('DELETE', '/api/performance/logs', 'root', 'CleanupLogFiles'),
    ('GET', '/api/performance/logs', 'root', 'GetLogFiles'),
    ('POST', '/api/performance/reset_stats', 'root', 'ResetPerformanceStats'),
    ('GET', '/api/performance/stats', 'root', 'GetPerformanceStats'),
    ('GET', '/api/plugin/task', 'root', 'ListTaskPlugins'),
    ('POST', '/api/plugin/task', 'root', 'UploadTaskPlugin'),
    ('PUT', '/api/plugin/task', 'root', 'UploadTaskPlugin'),
    ('GET', '/api/plugin/task/:key', 'root', 'GetTaskPlugin'),
    ('POST', '/api/plugin/task/:key/activate', 'root', 'ActivateTaskPlugin'),
    ('POST', '/api/plugin/task/:key/dryrun', 'root', 'DryRunTaskPlugin'),
    ('GET', '/api/plugin/task/:key/icon', 'root', 'GetTaskPluginIcon'),
    ('POST', '/api/plugin/task/:key/status', 'root', 'SetTaskPluginStatus'),
    ('GET', '/api/plugin/task/:key/versions', 'root', 'GetTaskPluginVersions'),
    ('DELETE', '/api/plugin/task/:key/versions/:version', 'root', 'DeleteTaskPluginVersion'),
    ('GET', '/api/plugin/task/marketplace/sources', 'root', 'GetTaskPluginMarketplaceSources'),
    ('PUT', '/api/plugin/task/marketplace/sources', 'root', 'UpdateTaskPluginMarketplaceSources'),
    ('GET', '/api/plugin/task/runtime/status', 'root', 'GetTaskPluginRuntime'),
    ('GET', '/api/prefill_group/', 'admin', 'GetPrefillGroups'),
    ('POST', '/api/prefill_group/', 'admin', 'CreatePrefillGroup'),
    ('PUT', '/api/prefill_group/', 'admin', 'UpdatePrefillGroup'),
    ('DELETE', '/api/prefill_group/:id', 'admin', 'DeletePrefillGroup'),
    ('POST', '/api/price-sentinel/edit/apply', 'root', 'ApplyPriceSentinelEdit'),
    ('POST', '/api/price-sentinel/edit/sync', 'root', 'SyncPriceSentinelPrices'),
    ('GET', '/api/price-sentinel/report', 'root', 'GetPriceSentinelReport'),
    ('POST', '/api/price-sentinel/run', 'root', 'RunPriceSentinelNow'),
    ('GET', '/api/proxy_pool/all', 'admin', 'ListProxyPoolProxies'),
    ('POST', '/api/proxy_pool/assign', 'admin', 'AssignProxyPoolProxies'),
    ('POST', '/api/proxy_pool/report_bad', 'admin', 'ReportBadProxyPoolProxy'),
    ('GET', '/api/proxy_pool/status', 'admin', 'GetProxyPoolStatus'),
    ('GET', '/api/ratio_sync/channels', 'root', 'GetSyncableChannels'),
    ('POST', '/api/ratio_sync/fetch', 'root', 'FetchUpstreamRatios'),
    ('GET', '/api/redemption/', 'admin', 'GetAllRedemptions'),
    ('POST', '/api/redemption/', 'admin', 'AddRedemption'),
    ('PUT', '/api/redemption/', 'admin', 'UpdateRedemption'),
    ('DELETE', '/api/redemption/:id', 'admin', 'DeleteRedemption'),
    ('GET', '/api/redemption/:id', 'admin', 'GetRedemption'),
    ('POST', '/api/redemption/batch', 'admin', 'DeleteRedemptionBatch'),
    ('DELETE', '/api/redemption/invalid', 'admin', 'DeleteInvalidRedemption'),
    ('GET', '/api/redemption/search', 'admin', 'SearchRedemptions'),
    ('GET', '/api/refund/admin/', 'admin', 'AdminListRefundRequests'),
    ('POST', '/api/refund/admin/:id/advance', 'admin', 'AdminAdvanceRefundRequest'),
    ('POST', '/api/refund/admin/cash', 'admin', 'AdminCreateCashRefundRequest'),
    ('POST', '/api/refund/admin/plan', 'admin', 'AdminCreatePlanRefundRequest'),
    ('GET', '/api/registrar/accounts', 'root', 'ListRegistrarAccounts'),
    ('GET', '/api/registrar/accounts/export', 'root', 'ExportRegistrarAccounts'),
    ('POST', '/api/registrar/accounts/import', 'root', 'ImportRegistrarAccounts'),
    ('GET', '/api/registrar/providers', 'root', 'GetRegistrarProviders'),
    ('PUT', '/api/registrar/providers', 'root', 'UpdateRegistrarProviders'),
    ('POST', '/api/registrar/providers/:id/run', 'root', 'RunRegistrarProvider'),
    ('GET', '/api/status/test', 'admin', 'TestStatus'),
    ('POST', '/api/subscription/admin/bind', 'admin', 'AdminBindSubscription'),
    ('GET', '/api/subscription/admin/plans', 'admin', 'AdminListSubscriptionPlans'),
    ('POST', '/api/subscription/admin/plans', 'admin', 'AdminCreateSubscriptionPlan'),
    ('DELETE', '/api/subscription/admin/plans/:id', 'admin', 'AdminDeleteSubscriptionPlan'),
    ('PATCH', '/api/subscription/admin/plans/:id', 'admin', 'AdminUpdateSubscriptionPlanStatus'),
    ('PUT', '/api/subscription/admin/plans/:id', 'admin', 'AdminUpdateSubscriptionPlan'),
    ('POST', '/api/subscription/admin/plans/:id/subscriptions/reset', 'admin', 'AdminResetPlanSubscriptions'),
    ('DELETE', '/api/subscription/admin/user_subscriptions/:id', 'admin', 'AdminDeleteUserSubscription'),
    ('POST', '/api/subscription/admin/user_subscriptions/:id/invalidate', 'admin', 'AdminInvalidateUserSubscription'),
    ('GET', '/api/subscription/admin/users/:id/subscriptions', 'admin', 'AdminListUserSubscriptions'),
    ('POST', '/api/subscription/admin/users/:id/subscriptions', 'admin', 'AdminCreateUserSubscription'),
    ('POST', '/api/subscription/admin/users/:id/subscriptions/reset', 'admin', 'AdminResetUserSubscriptionsByPlan'),
    ('GET', '/api/support/admin/conversations', 'admin', 'AdminListSupportConversations'),
    ('GET', '/api/support/admin/conversations/:id', 'admin', 'AdminGetSupportConversation'),
    ('POST', '/api/support/admin/conversations/:id/grant-quota', 'admin', 'AdminGrantSupportQuota'),
    ('POST', '/api/support/admin/conversations/:id/grant-subscription', 'admin', 'AdminGrantSupportSubscription'),
    ('POST', '/api/support/admin/conversations/:id/messages', 'admin', 'AdminSendSupportMessage'),
    ('POST', '/api/support/admin/messages/:id/complete-order', 'admin', 'AdminCompleteSupportOrder'),
    ('GET', '/api/system-info/instances', 'root', 'ListSystemInstances'),
    ('DELETE', '/api/system-info/instances/:node_name', 'root', 'DeleteStaleSystemInstance'),
    ('DELETE', '/api/system-info/stale-instances', 'root', 'DeleteStaleSystemInstances'),
    ('GET', '/api/system-task/:task_id', 'root', 'GetSystemTask'),
    ('GET', '/api/system-task/current', 'root', 'GetCurrentSystemTask'),
    ('DELETE', '/api/system-task/history', 'root', 'DeleteSystemTaskHistory'),
    ('GET', '/api/system-task/list', 'root', 'ListSystemTasks'),
    ('POST', '/api/system-task/log-cleanup', 'root', 'CreateLogCleanupSystemTask'),
    ('GET', '/api/task', 'admin', 'GetAllTask'),
    ('GET', '/api/task/export', 'admin', 'ExportTaskCSV'),
    ('GET', '/api/task_plugin_options', 'admin', 'GetTaskPluginOptions'),
    ('GET', '/api/token/group-names', 'admin', 'GetTokenGroupNames'),
    ('POST', '/api/token/group/migrate', 'admin', 'MigrateTokenGroup'),
    ('GET', '/api/user/', 'admin', 'GetAllUsers'),
    ('POST', '/api/user/', 'admin', 'CreateUser'),
    ('PUT', '/api/user/', 'admin', 'UpdateUser'),
    ('GET', '/api/user/2fa/stats', 'admin', 'Admin2FAStats'),
    ('DELETE', '/api/user/:id', 'admin', 'DeleteUser'),
    ('GET', '/api/user/:id', 'admin', 'GetUser'),
    ('DELETE', '/api/user/:id/2fa', 'admin', 'AdminDisable2FA'),
    ('DELETE', '/api/user/:id/bindings/:binding_type', 'admin', 'AdminClearUserBinding'),
    ('GET', '/api/user/:id/oauth/bindings', 'admin', 'GetUserOAuthBindingsByAdmin'),
    ('DELETE', '/api/user/:id/oauth/bindings/:provider_id', 'admin', 'UnbindCustomOAuthByAdmin'),
    ('DELETE', '/api/user/:id/reset_passkey', 'admin', 'AdminResetPasskey'),
    ('DELETE', '/api/user/:id/wallpaper', 'admin', 'AdminDeleteUserWallpaper'),
    ('GET', '/api/user/:id/wallpaper', 'admin', 'AdminGetUserWallpaper'),
    ('POST', '/api/user/manage', 'admin', 'ManageUser'),
    ('POST', '/api/user/manage/batch', 'admin', 'BatchManageUsers'),
    ('GET', '/api/user/search', 'admin', 'SearchUsers'),
    ('GET', '/api/user/topup', 'admin', 'GetAllTopUps'),
    ('POST', '/api/user/topup/complete', 'admin', 'AdminCompleteTopUp'),
    ('GET', '/api/vendors/', 'admin', 'GetAllVendors'),
    ('POST', '/api/vendors/', 'admin', 'CreateVendorMeta'),
    ('PUT', '/api/vendors/', 'admin', 'UpdateVendorMeta'),
    ('DELETE', '/api/vendors/:id', 'admin', 'DeleteVendorMeta'),
    ('GET', '/api/vendors/:id', 'admin', 'GetVendorMeta'),
    ('POST', '/api/vendors/operations', 'admin', 'ApplyVendorOperation'),
    ('POST', '/api/vendors/operations/preview', 'admin', 'PreviewVendorOperation'),
    ('GET', '/api/vendors/search', 'admin', 'SearchVendors'),
)

RETRY_STATUS = {429, 502, 503, 504}
RETRY_ATTEMPTS = 3


class ApiError(RuntimeError):
    """A non-2xx answer, kept with the status so callers can branch on it."""

    def __init__(self, status: int, message: str, body: object = None) -> None:
        super().__init__(f"HTTP {status}: {message}")
        self.status = status
        self.message = message
        self.body = body


class AdminClient:
    def __init__(
        self,
        base_url: str,
        token: str,
        timeout: float = 60.0,
        user_agent: str = DEFAULT_USER_AGENT,
    ) -> None:
        if not token:
            raise SystemExit(
                f"no admin token: pass --token or set {TOKEN_ENV} in the environment"
            )
        self.base_url = base_url.rstrip("/")
        self.token = token
        self.timeout = timeout
        self.user_agent = user_agent

    # -- transport ---------------------------------------------------------
    def request(
        self,
        method: str,
        path: str,
        query: dict[str, object] | None = None,
        data: object = None,
        *,
        timeout: float | None = None,
    ) -> object:
        """Call one endpoint and return the decoded body.

        Retries only the statuses that mean "try again", never a 4xx that the
        caller has to act on.
        """
        url = self.base_url + path
        if query:
            flat = {k: v for k, v in query.items() if v is not None}
            if flat:
                url += "?" + urllib.parse.urlencode(flat, doseq=True)

        payload = None
        headers = {
            "Authorization": self.token,
            "Accept": "application/json",
            "User-Agent": self.user_agent,
        }
        if data is not None:
            payload = json.dumps(data).encode("utf-8")
            headers["Content-Type"] = "application/json"

        if os.environ.get("REX_DEBUG"):
            # Field names and sizes only: a body can carry credentials (a
            # registrar import, a new token), so its values never reach the log.
            shape = ""
            if isinstance(data, dict):
                shape = " fields=" + ",".join(sorted(data)) 
            elif data is not None:
                shape = f" body={len(payload or b'')}B"
            print(f"[debug] {method} {url}{shape}", file=sys.stderr)

        last_error: Exception | None = None
        for attempt in range(1, RETRY_ATTEMPTS + 1):
            request = urllib.request.Request(
                url, data=payload, headers=headers, method=method.upper()
            )
            try:
                with urllib.request.urlopen(
                    request, timeout=timeout or self.timeout
                ) as response:
                    return self._decode(response.read(), response.status)
            except urllib.error.HTTPError as exc:
                body = exc.read()
                if exc.code in RETRY_STATUS and attempt < RETRY_ATTEMPTS:
                    last_error = exc
                    time.sleep(1.5 * attempt)
                    continue
                raise ApiError(exc.code, self._message(body), self._try_json(body)) from exc
            except (urllib.error.URLError, ssl.SSLError) as exc:
                # A reset mid-handshake (common on this network) is worth
                # another try; the error is only reported once the attempts run
                # out.
                if attempt < RETRY_ATTEMPTS:
                    last_error = exc
                    time.sleep(1.5 * attempt)
                    continue
                reason = getattr(exc, "reason", exc)
                raise ApiError(0, str(reason)) from exc
        raise ApiError(0, f"gave up after {RETRY_ATTEMPTS} attempts: {last_error}")

    @staticmethod
    def _try_json(body: bytes) -> object:
        try:
            return json.loads(body.decode("utf-8", "replace"))
        except (ValueError, UnicodeDecodeError):
            return None

    @classmethod
    def _decode(cls, body: bytes, status: int) -> object:
        if not body:
            return {"success": True, "status": status}
        parsed = cls._try_json(body)
        if parsed is None:
            return {"success": True, "status": status, "text": body.decode("utf-8", "replace")}
        return parsed

    @classmethod
    def _message(cls, body: bytes) -> str:
        parsed = cls._try_json(body)
        if isinstance(parsed, dict):
            for key in ("message", "error", "msg"):
                value = parsed.get(key)
                if isinstance(value, str) and value:
                    return value
        text = body.decode("utf-8", "replace").strip()
        return text[:400] if text else "(no message)"

    # -- shortcuts ---------------------------------------------------------
    def unwrap(self, response: object) -> object:
        """Return data from the {success, data} envelope the server uses."""
        if isinstance(response, dict) and "data" in response:
            return response["data"]
        return response

    def get(self, path: str, **query: object) -> object:
        return self.request("GET", path, query=query)

    def options(self) -> dict[str, str]:
        """Every setting as a flat key -> raw string map."""
        data = self.unwrap(self.request("GET", "/api/option/"))
        if isinstance(data, list):
            return {
                item["key"]: item.get("value")
                for item in data
                if isinstance(item, dict) and "key" in item
            }
        if isinstance(data, dict):
            return {str(k): v for k, v in data.items()}
        raise ApiError(0, "unexpected /api/option/ shape")

    def get_option(self, key: str) -> str | None:
        return self.options().get(key)

    def set_option(self, key: str, value: str) -> object:
        return self.request("PUT", "/api/option/", data={"key": key, "value": value})

    def channels(self, page_size: int = 1000) -> list[dict]:
        """Every channel.

        ``p=0`` alone returns the server's default page (10 here), which quietly
        truncates the list, so the page size is always sent explicitly.
        """
        data = self.unwrap(
            self.request("GET", "/api/channel/", query={"p": 1, "page_size": page_size})
        )
        if isinstance(data, dict) and isinstance(data.get("items"), list):
            return data["items"]
        if isinstance(data, list):
            return data
        return []

    def channel(self, channel_id: int) -> dict:
        return self.unwrap(self.request("GET", f"/api/channel/{channel_id}"))

    def save_channel(self, channel: dict, mode: str = "single") -> object:
        """Create when the payload has no id, patch when it does.

        Create takes a wrapped body ({"mode": ..., "channel": {...}}); update
        takes the channel itself, and because the server applies it with GORM's
        struct update only the fields present are written, so a partial patch
        leaves the key and everything else alone.
        """
        if channel.get("id"):
            return self.request("PUT", "/api/channel/", data=channel)
        return self.request(
            "POST", "/api/channel/", data={"mode": mode, "channel": channel}
        )

    def set_channel_status(self, channel_id: int, enabled: bool) -> object:
        return self.request(
            "POST", f"/api/channel/{channel_id}/status", data={"status": 1 if enabled else 2}
        )

    def channel_keys(self, channel_id: int) -> str:
        """The multi-key list. Root-only, and rate limited."""
        data = self.unwrap(self.request("POST", f"/api/channel/{channel_id}/key"))
        if isinstance(data, dict):
            return data.get("key") or data.get("keys") or ""
        return data if isinstance(data, str) else json.dumps(data)

    def redeem(self, **fields: object) -> object:
        return self.request("POST", "/api/redemption/", data=fields)

    def registrar_status(self) -> object:
        return self.unwrap(self.request("GET", "/api/registrar/providers"))

    def registrar_run(self, provider_id: str, count: int) -> object:
        return self.request(
            "POST", f"/api/registrar/providers/{provider_id}/run", data={"count": count}
        )

    def registrar_import(self, provider_id: str, content: str) -> object:
        return self.request(
            "POST", "/api/registrar/accounts/import", data={"provider_id": provider_id, "content": content}
        )


# ---------------------------------------------------------------------------
# route helpers
# ---------------------------------------------------------------------------
def find_routes(pattern: str | None, group: str | None, auth: str | None, method: str | None):
    out = []
    for route_method, path, route_auth, handler in ROUTES:
        if pattern and pattern.lower() not in path.lower():
            continue
        if group and not path.startswith(f"/api/{group.strip('/')}"):
            continue
        if auth and route_auth != auth:
            continue
        if method and route_method != method.upper():
            continue
        out.append((route_method, path, route_auth, handler))
    return out


def groups() -> list[tuple[str, int]]:
    counts: dict[str, int] = {}
    for _, path, _, _ in ROUTES:
        parts = [p for p in path.split("/") if p]
        name = parts[1] if len(parts) > 1 else "(root)"
        counts[name] = counts.get(name, 0) + 1
    return sorted(counts.items(), key=lambda kv: (-kv[1], kv[0]))


def parse_pairs(pairs: list[str] | None) -> dict[str, str]:
    out: dict[str, str] = {}
    for pair in pairs or []:
        if "=" not in pair:
            raise SystemExit(f"--field wants key=value, got {pair!r}")
        key, value = pair.split("=", 1)
        out[key] = value
    return out


def coerce(value: str) -> object:
    """Turn a CLI string into the JSON type it looks like, so numeric ids and
    booleans reach the server with the right type."""
    lowered = value.lower()
    if lowered in ("true", "false"):
        return lowered == "true"
    if lowered in ("null", "none"):
        return None
    try:
        return json.loads(value)
    except ValueError:
        return value


def force_utf8_stdio() -> None:
    """Make stdout able to carry whatever a setting value holds.

    A Windows console defaults to a legacy code page (GBK here) that cannot
    encode the emoji some settings store, which would abort a *successful*
    response at print time. Reconfiguring keeps the bytes valid UTF-8 for a pipe
    or a file even when the terminal itself renders them oddly.
    """
    for stream in (sys.stdout, sys.stderr):
        try:
            stream.reconfigure(encoding="utf-8")
        except (AttributeError, ValueError):
            pass


def emit(result: object, output: str) -> None:
    if output == "raw":
        text = json.dumps(result, ensure_ascii=False)
    else:
        text = json.dumps(result, ensure_ascii=False, indent=2, sort_keys=False)
    try:
        print(text)
    except UnicodeEncodeError:
        # Still encodable everywhere, at the cost of escaping non-ASCII.
        print(json.dumps(result, ensure_ascii=True, indent=2, sort_keys=False))


class UsageError(SystemExit):
    pass


# ---------------------------------------------------------------------------
# commands
# ---------------------------------------------------------------------------
def cmd_list(client: AdminClient, args: argparse.Namespace) -> object:
    if args.groups:
        return [{"group": name, "endpoints": count} for name, count in groups()]
    found = find_routes(args.filter, args.group, args.auth, args.method)
    if args.output == "table":
        width = max((len(p) for _, p, _, _ in found), default=0)
        for method, path, auth, _ in found:
            print(f"{method:<7} {path:<{width}}  {auth}")
        return None
    return [{"method": m, "path": p, "auth": a} for m, p, a, _ in found]


def normalize_path(raw: str) -> str:
    """Recover an API path that the shell may have rewritten.

    Git Bash (MSYS) rewrites an argument that looks like an absolute POSIX path
    into a Windows path before Python ever sees it, so ``/api/status`` arrives as
    ``D:/Git/api/status`` and the request would target a nonsense host. Cutting
    back to the ``/api/`` tail keeps one command working in cmd, PowerShell and
    Git Bash alike (``MSYS_NO_PATHCONV=1`` also avoids the rewrite).
    """
    candidate = raw.strip()
    if candidate.startswith("/"):
        return candidate
    marker = candidate.find("/api/")
    if marker != -1:
        return candidate[marker:]
    drive = re.match(r"^[A-Za-z]:[/\](.*)$", candidate)
    if drive:
        return "/" + drive.group(1).lstrip("/\\")
    return candidate


def cmd_call(client: AdminClient, args: argparse.Namespace) -> object:
    data = None
    if args.data_file:
        with open(args.data_file, encoding="utf-8") as handle:
            data = json.load(handle)
    elif args.data:
        data = json.loads(args.data)
    fields = parse_pairs(args.field)
    if fields:
        base = data if isinstance(data, dict) else {}
        data = {**base, **{k: coerce(v) for k, v in fields.items()}}
    return client.request(
        args.method,
        normalize_path(args.path),
        query=parse_pairs(args.query),
        data=data,
    )


def cmd_options(client: AdminClient, args: argparse.Namespace) -> object:
    all_options = client.options()
    if args.filter:
        needle = args.filter.lower()
        all_options = {k: v for k, v in all_options.items() if needle in k.lower()}
    return all_options


def cmd_option_get(client: AdminClient, args: argparse.Namespace) -> object:
    return {"key": args.key, "value": client.get_option(args.key)}


def cmd_option_set(client: AdminClient, args: argparse.Namespace) -> object:
    client.set_option(args.key, args.value)
    return {"key": args.key, "value": args.value, "written": True}


def cmd_channels(client: AdminClient, args: argparse.Namespace) -> object:
    items = client.channels()
    if args.enabled is not None:
        items = [c for c in items if bool(c.get("status") == 1) == args.enabled]
    if args.filter:
        needle = args.filter.lower()
        items = [
            c
            for c in items
            if needle in str(c.get("name", "")).lower()
            or needle in str(c.get("base_url", "")).lower()
            or needle in str(c.get("models", "")).lower()
        ]
    if args.brief:
        items = [
            {
                "id": c.get("id"),
                "name": c.get("name"),
                "status": c.get("status"),
                "group": c.get("group"),
                "base_url": c.get("base_url"),
                "models": len(str(c.get("models") or "").split(",")) if c.get("models") else 0,
            }
            for c in items
        ]
    return items


def cmd_channel_set_status(client: AdminClient, args: argparse.Namespace) -> object:
    client.set_channel_status(args.id, args.action == "enable")
    return {"id": args.id, "enabled": args.action == "enable"}


def cmd_channel_keys(client: AdminClient, args: argparse.Namespace) -> object:
    if args.action == "append":
        channel = client.channel(args.id)
        existing = str(channel.get("key") or "")
        parts = [p.strip() for p in existing.split("\n") if p.strip()]
        if args.key not in parts:
            parts.append(args.key)
        channel["key"] = "\n".join(parts)
        client.save_channel(channel)
        return {"id": args.id, "keys": len(parts), "appended": True}
    raw = client.channel_keys(args.id)
    return {"id": args.id, "count": len([p for p in raw.split("\n") if p.strip()])}


def cmd_registrar(client: AdminClient, args: argparse.Namespace) -> object:
    if args.action == "status":
        return client.registrar_status()
    if args.action == "run":
        return client.registrar_run(args.provider, args.count)
    if args.action == "import":
        with open(args.file, encoding="utf-8") as handle:
            content = handle.read()
        return client.registrar_import(args.provider, content)
    raise UsageError("registrar needs one of: status | run | import")


def cmd_redeem_create(client: AdminClient, args: argparse.Namespace) -> object:
    fields: dict[str, object] = {
        "name": args.name,
        "quota": args.quota,
        "count": args.count,
    }
    if args.expired_time is not None:
        fields["expired_time"] = args.expired_time
    return client.redeem(**fields)


def shared_options() -> argparse.ArgumentParser:
    """Common flags, attached to the top parser and to every subcommand.

    Defaults are SUPPRESS on the subcommand copy so that a flag given *before*
    the subcommand is not clobbered when the flag is absent after it.
    """
    shared = argparse.ArgumentParser(add_help=False)
    shared.add_argument("--base-url", default=argparse.SUPPRESS)
    shared.add_argument("--token", default=argparse.SUPPRESS)
    shared.add_argument("--timeout", type=float, default=argparse.SUPPRESS)
    shared.add_argument("--user-agent", default=argparse.SUPPRESS)
    shared.add_argument(
        "-o", "--output", choices=["json", "table", "raw"], default=argparse.SUPPRESS
    )
    return shared


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="admin_api.py",
        description="Manage every admin endpoint of a ReX-API instance with one token.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=(
            "examples:\n"
            "  admin_api.py list --group channel\n"
            "  admin_api.py options --filter register\n"
            "  admin_api.py option-set RegisterDailyLimit 60\n"
            "  admin_api.py channels --brief --filter dahl\n"
            "  admin_api.py call POST /api/channel/1/status --field status=2\n"
            "  admin_api.py call GET /api/log/ --query p=1 --query page_size=20\n"
        ),
    )
    parser.add_argument("--base-url", default=os.environ.get(BASE_URL_ENV, DEFAULT_BASE_URL))
    parser.add_argument("--token", default=os.environ.get(TOKEN_ENV, ""))
    parser.add_argument("--timeout", type=float, default=60.0)
    parser.add_argument("--user-agent", default=os.environ.get(UA_ENV, DEFAULT_USER_AGENT))
    parser.add_argument("-o", "--output", choices=["json", "table", "raw"], default="json")

    shared = shared_options()
    subs = parser.add_subparsers(dest="command", required=True)

    p = subs.add_parser("list", parents=[shared], help="every admin endpoint, or the group summary")
    p.add_argument("--group")
    p.add_argument("--auth", choices=["admin", "root"])
    p.add_argument("--method")
    p.add_argument("--groups", action="store_true", help="show the per-group counts")
    p.add_argument("filter", nargs="?", help="substring match on the path")
    p.set_defaults(func=cmd_list)

    p = subs.add_parser("call", parents=[shared], help="call any endpoint with an arbitrary body")
    p.add_argument("method")
    p.add_argument("path")
    p.add_argument("--query", action="append", help="k=v, repeatable")
    p.add_argument("--field", action="append", help="k=v merged into the JSON body, repeatable")
    p.add_argument("--data", help="JSON body")
    p.add_argument("--data-file", help="file holding the JSON body")
    p.set_defaults(func=cmd_call)

    p = subs.add_parser("options", parents=[shared], help="all settings, or those matching a filter")
    p.add_argument("--filter")
    p.set_defaults(func=cmd_options)

    p = subs.add_parser("option-get", parents=[shared], help="read one setting")
    p.add_argument("key")
    p.set_defaults(func=cmd_option_get)

    p = subs.add_parser("option-set", parents=[shared], help="write one setting")
    p.add_argument("key")
    p.add_argument("value")
    p.set_defaults(func=cmd_option_set)

    p = subs.add_parser("channels", parents=[shared], help="list channels, with optional filters")
    p.add_argument("--filter")
    p.add_argument("--brief", action="store_true", help="id, name, status, group, model count only")
    p.add_argument("--enabled", dest="enabled", action="store_true", default=None)
    p.add_argument("--disabled", dest="enabled", action="store_false")
    p.set_defaults(func=cmd_channels)

    p = subs.add_parser("channel", parents=[shared], help="one channel, or flip its status")
    p.add_argument("action", choices=["get", "enable", "disable"])
    p.add_argument("id", type=int)
    p.set_defaults(func=lambda c, a: c.channel(a.id) if a.action == "get" else cmd_channel_set_status(c, a))

    p = subs.add_parser("channel-keys", parents=[shared], help="read or append a multi-key channel's keys")
    p.add_argument("action", choices=["count", "append"])
    p.add_argument("id", type=int)
    p.add_argument("--key", help="required with append")
    p.set_defaults(func=cmd_channel_keys)

    p = subs.add_parser("registrar", parents=[shared], help="registrar providers and their account store")
    p.add_argument("action", choices=["status", "run", "import"])
    p.add_argument("--provider", default="dahl")
    p.add_argument("--count", type=int, default=1)
    p.add_argument("--file", help="dahl_keys.txt-style archive, required with import")
    p.set_defaults(func=cmd_registrar)

    p = subs.add_parser("redeem-create", parents=[shared], help="mint redemption codes")
    p.add_argument("--name", required=True)
    p.add_argument("--quota", type=int, required=True)
    p.add_argument("--count", type=int, default=1)
    p.add_argument("--expired-time", type=int, default=None, dest="expired_time")
    p.set_defaults(func=cmd_redeem_create)

    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    force_utf8_stdio()
    client = AdminClient(args.base_url, args.token, args.timeout, args.user_agent)
    try:
        result = args.func(client, args)
    except ApiError as exc:
        print(f"error: {exc.message}", file=sys.stderr)
        if exc.body is not None:
            print(json.dumps(exc.body, ensure_ascii=False, indent=2), file=sys.stderr)
        return 1
    except UsageError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2
    if result is not None:
        emit(result, args.output)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
