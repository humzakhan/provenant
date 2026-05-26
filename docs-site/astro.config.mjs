import { defineConfig } from 'astro/config';
import mdx from '@astrojs/mdx';
import starlight from '@astrojs/starlight';

export default defineConfig({
  integrations: [
    starlight({
      title: 'Provenant Docs',
      customCss: ['./src/styles/provenant.css'],
      sidebar: [
        {
          label: 'Orientation',
          items: [
            { label: 'Home', link: '/' },
            { label: 'Guarantees and Limits', link: '/guarantees-and-limits/' },
          ],
        },
        {
          label: 'Current Artifact Quickstart',
          items: [{ label: 'Run Current Artifacts', link: '/quickstart/current-artifacts/' }],
        },
        {
          label: 'Reference',
          items: [{ label: 'Status Matrix', link: '/reference/status-matrix/' }],
        },
      ],
    }),
    mdx(),
  ],
});
