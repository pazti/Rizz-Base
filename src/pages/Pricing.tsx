import type { PageProps } from "../types";
import { Footer, Header } from "../components/layout";
import { BigBanner, ComingSoonNotice, Icon } from "../components/ui";

export default function Pricing({ theme, toggle, go }: PageProps) {
  return (
    <>
      <Header theme={theme} toggle={toggle} go={go} />
      <ComingSoonNotice message="Rizzbase V1 is a local library, not a hosted service. There are no subscriptions, accounts, or hosted plans." />
      <main className="pricing-page">
        <div className="pricing-head"><span>LOCAL-FIRST V1</span><h1>NO HOSTED<br /><mark>PLANS OR FEES.</mark></h1><p>Use the published Node.js SDK to work with encrypted Project files on your device.</p><a className="btn primary" href="https://www.npmjs.com/package/rizzbase" target="_blank" rel="noreferrer">GET THE SDK <Icon name="arrow" /></a></div>
        <BigBanner />
      </main>
      <Footer go={go} />
    </>
  );
}
