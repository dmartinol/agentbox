import { Tabs } from 'radix-ui';
import { Sheet } from '../components/ui/sheet';
import { TabContent, TabList, TabTrigger } from '../components/ui/tabs';
import { ProvidersPanel } from './setup/ProvidersPanel';
import { ProfilesPanel } from './setup/ProfilesPanel';
import { ProjectsPanel } from './setup/ProjectsPanel';
import { ImagesPanel } from './setup/ImagesPanel';
import { GatewaysPanel } from './setup/GatewaysPanel';
import { PoliciesPanel } from './setup/PoliciesPanel';
import { McpPanel } from './setup/McpPanel';
import { AdvancedPanel } from './setup/AdvancedPanel';
import type { LogDrawerState } from '../components/LogDrawer';

export function SetupSheet({
  open,
  onOpenChange,
  openLog,
  updateLog,
  onResourcesChanged
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  openLog: (state: Omit<LogDrawerState, 'open'>) => void;
  updateLog: (state: Partial<LogDrawerState>) => void;
  onResourcesChanged?: () => void;
}) {
  return (
    <Sheet open={open} onOpenChange={onOpenChange} title="Setup" description="Projects, profiles, providers, images, gateways, policies, MCP tools, and diagnostics" width="58rem">
      <Tabs.Root defaultValue="providers" className="flex h-full min-h-0 flex-col">
        <TabList>
          <TabTrigger value="providers">Providers</TabTrigger>
          <TabTrigger value="profiles">Profiles</TabTrigger>
          <TabTrigger value="projects">Projects</TabTrigger>
          <TabTrigger value="images">Images</TabTrigger>
          <TabTrigger value="gateways">Gateways</TabTrigger>
          <TabTrigger value="policies">Policies</TabTrigger>
          <TabTrigger value="mcps">MCP Tools</TabTrigger>
          <TabTrigger value="advanced">Advanced</TabTrigger>
        </TabList>
        <TabContent value="providers"><ProvidersPanel /></TabContent>
        <TabContent value="profiles"><ProfilesPanel onChanged={onResourcesChanged} /></TabContent>
        <TabContent value="projects"><ProjectsPanel onChanged={onResourcesChanged} /></TabContent>
        <TabContent value="images"><ImagesPanel openLog={openLog} updateLog={updateLog} /></TabContent>
        <TabContent value="gateways"><GatewaysPanel /></TabContent>
        <TabContent value="policies"><PoliciesPanel /></TabContent>
        <TabContent value="mcps"><McpPanel /></TabContent>
        <TabContent value="advanced"><AdvancedPanel /></TabContent>
      </Tabs.Root>
    </Sheet>
  );
}
