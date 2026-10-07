import { createFileRoute } from '@tanstack/react-router'

import { RechargeFormCard } from '@/features/wallet/components/recharge-form-card'
import { RedemptionShopCard } from '@/features/wallet/components/redemption-shop-card'

export const Route = createFileRoute('/shop-preview')({
  component: ShopPreview,
})

function ShopPreview() {
  return (
    <div className='mx-auto flex w-full max-w-7xl flex-col gap-4 p-6 sm:gap-5'>
      <RedemptionShopCard />
      <RechargeFormCard
        topupInfo={null}
        presetAmounts={[]}
        selectedPreset={null}
        onSelectPreset={() => {}}
        topupAmount={0}
        onTopupAmountChange={() => {}}
        paymentAmount={0}
        calculating={false}
        onPaymentMethodSelect={() => {}}
        paymentLoading={null}
        redemptionCode=''
        onRedemptionCodeChange={() => {}}
        onRedeem={() => {}}
        redeeming={false}
      />
    </div>
  )
}
