# COMKUNAI 六槽观摩历史策略

这六套策略只展示经过校验的 `historical_simulation` 历史行情场景数据，
不代表实盘已实现收益，也没有绑定独立实时主控。启用开关：
`COMKUN_OBSERVER_MARKET_ENABLED=true`。

| Slot | Strategy ID | Profile | Display name |
| --- | --- | --- | --- |
| 1 | `comkun-observation-history-01` | `steady-22` | 稳衡一号 |
| 2 | `comkun-observation-history-02` | `tide-35` | 潮汐二号 |
| 3 | `comkun-observation-history-03` | `arc-37` | 弧光三号 |
| 4 | `comkun-observation-history-04` | `orbit-46` | 星轨四号 |
| 5 | `comkun-observation-history-05` | `summit-53` | 峻峰五号 |
| 6 | `comkun-observation-history-06` | `pulse-109` | 脉冲六号 |

历史来源账号只作为 provenance，不能用作 `masterAccountId`。当前实时链路继续使用
单独的 AI master 与三个 HZ follower；六套策略的
`realtime_follow_available` 均为 `false`。

API 分开返回 `completed_months` 与 `monthly_rows`。后者包含当月进行中记录，
不得把它计作完整月份。

内置数据包 SHA256：
`375b879fdb89549a1612a1486d7933098a59fe40f30bf0508679a4b7996e06d3`。
