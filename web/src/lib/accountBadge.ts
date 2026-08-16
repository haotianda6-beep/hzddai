export type AccountPartnerRole = 'retail' | 'studio' | 'branch'

const partnerRoleName: Record<AccountPartnerRole, string> = {
  retail: '散户',
  studio: '工作室',
  branch: '分公司',
}

export function getAccountBadgeName(
  isAdmin: boolean,
  partnerRole?: AccountPartnerRole
): string | undefined {
  if (isAdmin) return '超级管理员'
  return partnerRole ? partnerRoleName[partnerRole] : undefined
}
