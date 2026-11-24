<script setup lang="ts">
import { NConfigProvider, NDialogProvider, NMessageProvider, dateZhCN, zhCN } from 'naive-ui'

/**
 * 播放器页面保活（参考 zyronon/douyin 等同类项目的条件缓存方案）：
 * 从浏览页/收藏播放页切到设置、收藏夹、后台时，组件实例与 <video> 元素都保留，
 * 返回时直接恢复同一个播放器与播放位置，不再依赖 sessionStorage 快照重建页面。
 */
const KEPT_VIEWS = ['FeedView', 'FavoritesPlayView']
</script>

<template>
  <n-config-provider :locale="zhCN" :date-locale="dateZhCN">
    <n-dialog-provider>
      <n-message-provider>
        <router-view v-slot="{ Component, route }">
          <keep-alive :include="KEPT_VIEWS" :max="4">
            <component :is="Component" :key="String(route.name ?? route.path)" />
          </keep-alive>
        </router-view>
      </n-message-provider>
    </n-dialog-provider>
  </n-config-provider>
</template>
