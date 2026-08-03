export type AccountPartnerRole = 'retail' | 'ib' | 'studio' | 'branch'

const partnerRoleName: Record<AccountPartnerRole, string> = {
  retail: '散户',
  ib: 'IB',
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
