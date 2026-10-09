/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
// ----------------------------------------------------------------------------
// Pricing Types
// ----------------------------------------------------------------------------

export type PricingVendor = {
  id: number
  name: string
  icon?: string
  description?: string
}

export type BillingUsageUnit = 'second' | 'count' | 'token' | 'credit'

export type BillingUsageFieldSchema = {
  type?: 'number' | 'boolean'
  unit?: BillingUsageUnit
  unitLabel?: string | Record<string, string>
  enum?: string[]
  enumLabels?: Record<string, string | Record<string, string>>
  description?: string | Record<string, string>
}

export type BillingUsageSchema = Record<string, BillingUsageFieldSchema>

export type BillingUsageExample = {
  label: string
  facts: Record<string, string | number>
}

export type BillingPluginVariant = {
  plugin_key: string
  plugin_name: string
  icon?: string
  billing_expr: string
  billing_mode?: 'ratio' | 'tiered_expr'
  billing_usage_schema: BillingUsageSchema
  billing_usage_examples?: BillingUsageExample[]
}

export type PricingModel = {
  billing_plugin_variants?: BillingPluginVariant[]
  id: number
  model_name: string
  description?: string
  icon?: string
  vendor_id?: number
  vendor_name?: string
  vendor_icon?: string
  vendor_description?: string
  quota_type: number
  // Absent when the model has no configured ratio, which is not the same as
  // a zero price: an unpriced model must not render as free.
  model_ratio?: number
  completion_ratio: number
  model_price?: number
  cache_ratio?: number | null
  create_cache_ratio?: number | null
  image_ratio?: number | null
  audio_ratio?: number | null
  audio_completion_ratio?: number | null
  enable_groups: string[]
  tags?: string
  supported_endpoint_types?: string[]
  key?: string
  group_ratio?: Record<string, number>
  /** Billing mode (e.g. "tiered_expr") used to flag dynamic pricing */
  billing_mode?: string
  /** Raw expression describing dynamic / tiered billing */
  billing_expr?: string
  /** Task-plugin usage facts and their billing units. */
  billing_usage_schema?: BillingUsageSchema
  /** Display-only labeled usage vectors for pricing examples. */
  billing_usage_examples?: BillingUsageExample[]
  /** Pricing version returned by backend, useful for cache busting */
  pricing_version?: string
  /**
   * Optional model metadata fields reserved for backend-provided catalog data.
   * Keep them data-driven; do not synthesize display values on the client.
   */
  context_length?: number
  max_output_tokens?: number
  knowledge_cutoff?: string
  release_date?: string
  parameter_count?: string
  input_modalities?: Modality[]
  output_modalities?: Modality[]
  capabilities?: ModelCapability[]
}

/** Input/output modalities supported by a model. */
export type Modality = 'text' | 'image' | 'audio' | 'video' | 'file'

/** Functional capabilities a model exposes. */
export type ModelCapability =
  | 'function_calling'
  | 'streaming'
  | 'vision'
  | 'json_mode'
  | 'structured_output'
  | 'reasoning'
  | 'tools'
  | 'system_prompt'
  | 'web_search'
  | 'code_interpreter'
  | 'caching'
  | 'embeddings'

export type PricingData = {
  success: boolean
  message?: string
  data: PricingModel[]
  vendors: PricingVendor[]
  group_ratio: Record<string, number>
  /**
   * Which pricing source each group applies to each model, as
   * `{group: {model: "ratio" | "price" | "tiered_expr"}}`. A group absent from
   * this map leaves every model on its global billing mode, which is the same
   * shape the backend option stores.
   *
   * The square needs it because a group ratio is only a multiplier: without the
   * source, a model billed per call in one group and per token in another would
   * show the same figure in both.
   */
  group_billing_source?: Record<string, Record<string, string>>
  /**
   * Per-token ratio override by group, as `{group: {model: ratio}}`.
   *
   * A model can be fed by a different upstream in each group, so the same name
   * costs a different amount per token depending on which group serves it. The
   * override replaces the model's global `model_ratio` before the group ratio
   * multiplies it, which is why the square has to read it to show the price a
   * viewer would actually pay.
   */
  group_model_ratio?: Record<string, Record<string, number>>
  /**
   * Per-group output/input and cache/input multipliers, as
   * `{group: {model: ratio}}`. Two upstreams can quote the same model with
   * different output and cache ratios, and the global multipliers only fit one
   * of them, so the square has to read these to price the other group right.
   */
  group_model_completion_ratio?: Record<string, Record<string, number>>
  group_model_cache_ratio?: Record<string, Record<string, number>>
  usable_group: Record<string, { desc: string; ratio: number }>
  model_square_group_descriptions?: Record<string, string>
  supported_endpoint: Record<string, string>
  auto_groups: string[]
  model_square_groups?: string[]
}

export type TokenUnit = 'M' | 'K'
export type PriceType =
  | 'input'
  | 'output'
  | 'cache'
  | 'create_cache'
  | 'image'
  | 'audio_input'
  | 'audio_output'
export type QuotaType = 0 | 1 // 0: token-based, 1: per-request
