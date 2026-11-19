import type { PageProps } from "../types";
import { Footer, Header } from "../components/layout";
import { BigBanner, ComingSoonNotice, Icon } from "../components/ui";

export default function Pricing({ theme, toggle, go }: PageProps) {
  return (
    <>
      <Header theme={theme} toggle={toggle} go={go} />
      <ComingSoonNotice message="Rizzbase V1 is a local library, not a hosted service. There are no subscriptions, accounts, or hosted plans." />
      <main className="pricing-page">
        <div className="pricing-head"><span>LOCAL-FIRST V1</span><h1>NO HOSTED<br /><mark>PLANS OR FEES.</mark></h1><p>Use the official Go SDK or the published Node.js and Python SDKs to work with encrypted Project files on your device.</p><button className="btn primary" onClick={() => go("docs")}>VIEW SDK DOCS <Icon name="arrow" /></button></div>
        <BigBanner />
      </main>
      <Footer go={go} />
    </>
  );
}
