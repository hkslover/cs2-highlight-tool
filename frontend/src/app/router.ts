import { createRouter, createWebHashHistory } from "vue-router";
import type { RouteRecordRaw } from "vue-router";
import { useDebugSettings } from "@/shared/state/useDebugSettings";

const routes: RouteRecordRaw[] = [
  {
    path: "/",
    redirect: "/import",
  },
  {
    path: "/import",
    component: () => import("@/features/import/pages/ImportPage.vue"),
    children: [
      {
        path: "",
        name: "import-actions",
        component: () => import("@/features/import/pages/ImportActions.vue"),
      },
      {
        path: "wanmei",
        name: "import-wanmei",
        component: () => import("@/features/import/pages/WanmeiImport.vue"),
      },
      {
        path: "5e",
        name: "import-5e",
        component: () => import("@/features/import/pages/FiveEImport.vue"),
      },
    ],
  },
  {
    path: "/clips",
    name: "clips",
    component: () => import("@/features/clips/pages/ClipsPage.vue"),
  },
  {
    path: "/produce",
    name: "produce",
    component: () => import("@/features/produce/pages/ProducePage.vue"),
  },
  {
    path: "/edit",
    name: "edit",
    component: () => import("@/features/edit/pages/EditPage.vue"),
  },
  {
    path: "/debug",
    name: "debug",
    component: () => import("@/features/debug/pages/DebugPage.vue"),
  },
  {
    path: "/settings",
    name: "settings",
    component: () => import("@/features/settings/pages/SettingsPage.vue"),
  },
];

const router = createRouter({
  history: createWebHashHistory(),
  routes,
});

// The debug workbench only exists after the hidden brand-click activation.
// A direct hash entry without it must land on the normal import step instead
// of rendering a page whose backend methods are not available. Match the route
// record instead of `to.path`, because `/debug/` resolves to the same record.
router.beforeEach((to) => {
  const isDebugRoute = to.matched.some((record) => record.name === "debug");
  if (isDebugRoute && !useDebugSettings().debugEnabled.value) {
    return { path: "/import" };
  }
  return true;
});

export default router;
