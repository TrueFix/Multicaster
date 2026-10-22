import {
  ArrowRight,
  Check,
  HelpCircle,
  MonitorPlay,
  Plus,
  Radio,
} from "lucide-react";
import { Button } from "@/components/ui/button";

export default function GettingStarted({
  live,
  hasPlatforms,
  onSettings,
  onAdd,
}) {
  return (
    <section
      className="getting-started"
      aria-labelledby="getting-started-title"
    >
      <div className="guide-heading">
        <div>
          <h2 id="getting-started-title">First stream? Start here.</h2>
          <p>
            OBS makes the video. Multicaster sends it to the places you choose.
          </p>
        </div>
        <span className="guide-badge">
          <HelpCircle size={16} />
          No streaming experience needed
        </span>
      </div>
      <ol className="setup-steps">
        <li>
          <span className={"step-number " + (live ? "complete" : "")}>
            {live ? <Check size={19} /> : 1}
          </span>
          <div>
            <h3>Connect OBS</h3>
            <p>
              OBS is the free app that captures your game or camera. Copy your
              Multicaster server URL and stream key into OBS.
            </p>
            <Button variant="link" onClick={onSettings}>
              Get my OBS details
              <ArrowRight size={16} />
            </Button>
          </div>
        </li>
        <li>
          <span className={"step-number " + (hasPlatforms ? "complete" : "")}>
            {hasPlatforms ? <Check size={19} /> : 2}
          </span>
          <div>
            <h3>Add a platform</h3>
            <p>
              Choose YouTube, Twitch, or another service. We’ll explain where to
              find its server URL and stream key.
            </p>
            <Button variant="link" onClick={onAdd}>
              Add my platform
              <Plus size={16} />
            </Button>
          </div>
        </li>
        <li>
          <span
            className={
              "step-number " + (live && hasPlatforms ? "complete" : "")
            }
          >
            {live && hasPlatforms ? <Check size={19} /> : 3}
          </span>
          <div>
            <h3>Start streaming in OBS</h3>
            <p>
              Enable “Restream to platforms” below, turn on your platforms, then
              click <strong>Start Streaming</strong> in OBS.
            </p>
            <span className="setup-footnote">
              <Radio size={16} />
              {live
                ? "OBS is connected. Check your platform status below."
                : "We’ll show “Live” when your video starts arriving."}
            </span>
          </div>
        </li>
      </ol>
    </section>
  );
}

export function StreamingHelp() {
  return (
    <details className="help-panel">
      <summary>
        <HelpCircle size={18} />
        What do these streaming terms mean?
      </summary>
      <dl className="glossary">
        <div>
          <dt>Server URL</dt>
          <dd>
            The address where a video stream is sent. A platform’s address sends
            video to that platform; your Multicaster address receives video from
            OBS.
          </dd>
        </div>
        <div>
          <dt>Stream key</dt>
          <dd>
            A private code that tells a server which channel the video belongs
            to. Your Multicaster key and your YouTube or Twitch key are
            different.
          </dd>
        </div>
        <div>
          <dt>RTMP / RTMPS</dt>
          <dd>
            Ways to send live video to a server. RTMPS adds encryption. Use the
            address your platform provides.
          </dd>
        </div>
        <div>
          <dt>Web stream / HLS</dt>
          <dd>
            A version of your live stream that can play in a browser. It usually
            plays several seconds behind OBS.
          </dd>
        </div>
        <div>
          <dt>Bitrate</dt>
          <dd>
            The amount of video data arriving each second. A higher number
            usually means a sharper picture and more internet bandwidth used.
          </dd>
        </div>
        <div>
          <dt>Passthrough</dt>
          <dd>
            Multicaster forwards the video OBS already prepared, instead of
            encoding it again. This keeps processing work low.
          </dd>
        </div>
      </dl>
    </details>
  );
}
