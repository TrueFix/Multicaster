import { useState } from "react";
import {
  ArrowUpCircle,
  Heart,
  LayoutDashboard,
  LockKeyhole,
  Menu,
  Radio,
  Settings2,
  Sparkles,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from "@/components/ui/sheet";

const pages = [
  {
    id: "overview",
    title: "Overview",
    icon: LayoutDashboard,
    description: "Your live stream at a glance",
  },
  {
    id: "platforms",
    title: "My platforms",
    icon: Radio,
    description: "Choose where your audience watches",
  },
  {
    id: "settings",
    title: "Settings",
    icon: Settings2,
    description: "Connect OBS and make yourself at home",
  },
];
export default function DashboardFrame({
  page,
  onNavigate,
  onLogout,
  offline,
  children,
}) {
  const [open, setOpen] = useState(false);
  const navigate = (id) => {
    onNavigate(id);
    setOpen(false);
  };
  return (
    <div className="dashboard-wrap">
      <div className="workspace-caption">
        <span>
          <Sparkles size={16} />
          ONE STREAM. A FEW MORE POSSIBILITIES.
        </span>
        <span className={offline ? "text-destructive" : "connection-good"}>
          <i />
          {offline
            ? "Reconnecting to your server…"
            : "Connected to your server"}
        </span>
      </div>
      <div className="dashboard-frame">
        <header className="app-header">
          <button
            className="app-brand"
            onClick={() => navigate("overview")}
            aria-label="Multicaster home"
          >
            <span className="brand-mark">
              <Sparkles size={23} />
            </span>
            <span>
              multicaster<span className="text-primary">.</span>
            </span>
          </button>
          <nav className="desktop-nav" aria-label="Main navigation">
            {pages.map((item) => (
              <Button
                key={item.id}
                variant="ghost"
                aria-current={page === item.id ? "page" : undefined}
                onClick={() => navigate(item.id)}
              >
                <item.icon size={18} />
                {item.title}
              </Button>
            ))}
          </nav>
          <div className="flex items-center gap-2">
            <Button
              variant="ghost"
              size="icon"
              aria-label="Lock dashboard"
              title="Lock dashboard"
              onClick={onLogout}
            >
              <LockKeyhole size={19} />
            </Button>
            <Button
              variant="outline"
              size="icon"
              className="mobile-menu-trigger"
              aria-label="Open navigation"
              onClick={() => setOpen(true)}
            >
              <Menu />
            </Button>
          </div>
        </header>
        {children}
      </div>
      <p className="below-frame">
        <Heart size={15} />
        One small app. Your stream, your way.
      </p>
      <Sheet open={open} onOpenChange={setOpen}>
        <SheetContent side="left" className="mobile-navigation">
          <SheetHeader>
            <SheetTitle className="app-brand">
              <span className="brand-mark">
                <Sparkles />
              </span>
              multicaster.
            </SheetTitle>
            <SheetDescription>
              Everything you need to share your stream.
            </SheetDescription>
          </SheetHeader>
          <nav aria-label="Mobile navigation" className="mobile-nav-list">
            {pages.map((item) => (
              <button
                key={item.id}
                aria-current={page === item.id ? "page" : undefined}
                onClick={() => navigate(item.id)}
              >
                <span className="icon-tile">
                  <item.icon />
                </span>
                <span>
                  <strong>{item.title}</strong>
                  <small>{item.description}</small>
                </span>
              </button>
            ))}
          </nav>
        </SheetContent>
      </Sheet>
    </div>
  );
}
