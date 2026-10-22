import * as React from "react";
import {
  LayoutDashboard,
  Radio,
  Settings2,
  Sparkles,
  ArrowUpCircle,
  Heart,
  LogOut,
} from "lucide-react";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from "@/components/ui/sidebar";
import {
  Card,
  CardContent,
  CardDescription,
  CardTitle,
} from "@/components/ui/card";
import { Button } from "@/components/ui/button";
const navigation = [
  { id: "overview", name: "Overview", icon: LayoutDashboard },
  { id: "platforms", name: "My platforms", icon: Radio },
  { id: "settings", name: "Settings", icon: Settings2 },
  { id: "updates", name: "Updates", icon: ArrowUpCircle },
];
export function AppSidebar({ page, onNavigate, onLogout, ...props }: any) {
  const { setOpenMobile } = useSidebar();
  return (
    <Sidebar collapsible="icon" variant="inset" {...props}>
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton
              asChild
              className="hover:text-foreground h-10 group-data-[collapsible=icon]:px-0!"
            >
              <a href="#overview" onClick={() => onNavigate("overview")}>
                <span className="brand-mark">
                  <Sparkles size={20} />
                </span>
                <span className="text-foreground font-semibold">
                  multicaster<span className="brand-dot">.</span>
                </span>
              </a>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
        <p className="sidebar-caption group-data-[collapsible=icon]:hidden">
          A little stream magic
        </p>
      </SidebarHeader>
      <SidebarContent>
        <SidebarMenu className="px-2 py-4">
          {navigation.map((item) => (
            <SidebarMenuItem key={item.id}>
              <SidebarMenuButton
                isActive={page === item.id}
                tooltip={item.name}
                onClick={() => {
                  onNavigate(item.id);
                  setOpenMobile(false);
                }}
              >
                <item.icon />
                <span>{item.name}</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          ))}
        </SidebarMenu>
      </SidebarContent>
      <SidebarFooter>
        <Card className="overflow-hidden group-data-[collapsible=icon]:hidden">
          <CardContent className="space-y-3 p-3">
            <div className="space-y-1">
              <Heart className="text-primary mb-3" size={21} />
              <CardTitle>Small footprint. Big reach.</CardTitle>
              <CardDescription>
                Encode once in OBS. We'll take your stream from here.
              </CardDescription>
            </div>
            <Button
              className="w-full"
              variant="secondary"
              onClick={() => onNavigate("settings")}
            >
              Make yourself at home
            </Button>
          </CardContent>
        </Card>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton onClick={onLogout}>
              <LogOut />
              <span>Lock dashboard</span>
            </SidebarMenuButton>
          </SidebarMenuItem>
        </SidebarMenu>
        <p className="sidebar-caption group-data-[collapsible=icon]:hidden">
          Self-hosted · Made for your stream
        </p>
      </SidebarFooter>
    </Sidebar>
  );
}
